// Package bookings implements the fake payment step, booking confirmation, booking history
// and cancellation.
package bookings

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"yozigo/internal/apperr"
	"yozigo/internal/database"
	"yozigo/internal/seats"
)

// CancellationCutoff: bookings can be cancelled until this long before the show starts.
const CancellationCutoff = 2 * time.Hour

type Booking struct {
	ID            int64      `json:"id"`
	ShowID        int64      `json:"showId"`
	MovieID       int64      `json:"movieId"`
	MovieTitle    string     `json:"movieTitle"`
	StartTime     time.Time  `json:"startTime"`
	EndTime       time.Time  `json:"endTime"`
	Seats         []string   `json:"seats"`
	TotalPaise    int        `json:"totalPaise"`
	Status        string     `json:"status"` // CONFIRMED or CANCELLED
	CreatedAt     time.Time  `json:"createdAt"`
	CancelledAt   *time.Time `json:"cancelledAt"`
	Cancellable   bool       `json:"cancellable"`
	CustomerName  string     `json:"customerName,omitempty"` // admin views only
	CustomerEmail string     `json:"customerEmail,omitempty"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const bookingSelect = `
	SELECT b.id, b.show_id, s.movie_id, m.title, s.start_time, s.end_time,
	       array_agg(se.row_label || se.seat_number::text ORDER BY se.id) AS seats,
	       b.total_paise, b.status, b.created_at, b.cancelled_at,
	       (b.status = 'CONFIRMED' AND s.start_time - make_interval(hours => 2) > now()) AS cancellable,
	       u.name, u.email
	FROM bookings b
	JOIN shows s ON s.id = b.show_id
	JOIN movies m ON m.id = s.movie_id
	JOIN users u ON u.id = b.user_id
	JOIN booking_seats bs ON bs.booking_id = b.id
	JOIN seats se ON se.id = bs.seat_id`

const bookingGroupBy = ` GROUP BY b.id, s.id, m.id, u.id`

func scanBooking(row pgx.Row) (*Booking, error) {
	var b Booking
	err := row.Scan(&b.ID, &b.ShowID, &b.MovieID, &b.MovieTitle, &b.StartTime, &b.EndTime,
		&b.Seats, &b.TotalPaise, &b.Status, &b.CreatedAt, &b.CancelledAt, &b.Cancellable,
		&b.CustomerName, &b.CustomerEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.New(apperr.NotFound, "BOOKING_NOT_FOUND", "Booking not found.")
	}
	return &b, err
}

func collect(rows pgx.Rows, err error, keepCustomer bool) ([]Booking, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Booking{}
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		if !keepCustomer {
			b.CustomerName, b.CustomerEmail = "", ""
		}
		list = append(list, *b)
	}
	return list, rows.Err()
}

// ListForUser returns the user's bookings: upcoming shows first (soonest first), then past
// shows (most recent first). Ownership is part of the WHERE clause.
func (s *Store) ListForUser(ctx context.Context, userID int64) ([]Booking, error) {
	rows, err := s.pool.Query(ctx, bookingSelect+`
		WHERE b.user_id = $1`+bookingGroupBy+`
		ORDER BY (s.start_time <= now()),
		         CASE WHEN s.start_time > now() THEN s.start_time END ASC,
		         s.start_time DESC, b.id DESC`, userID)
	return collect(rows, err, false)
}

// GetForUser returns one booking if, and only if, it belongs to userID. Someone else's booking
// is reported as "not found" so booking ids cannot be probed.
func (s *Store) GetForUser(ctx context.Context, userID, bookingID int64) (*Booking, error) {
	b, err := scanBooking(s.pool.QueryRow(ctx, bookingSelect+`
		WHERE b.id = $1 AND b.user_id = $2`+bookingGroupBy, bookingID, userID))
	if err != nil {
		return nil, err
	}
	b.CustomerName, b.CustomerEmail = "", ""
	return b, nil
}

// ListForShow returns every booking of a show with customer details (admin only).
func (s *Store) ListForShow(ctx context.Context, showID int64) ([]Booking, error) {
	rows, err := s.pool.Query(ctx, bookingSelect+`
		WHERE b.show_id = $1`+bookingGroupBy+` ORDER BY b.created_at, b.id`, showID)
	return collect(rows, err, true)
}

var errHoldNotFound = apperr.New(apperr.NotFound, "HOLD_NOT_FOUND", "Hold not found.")

func holdExpired() *apperr.Error {
	return apperr.New(apperr.Gone, "HOLD_EXPIRED", "Your seat hold has expired. Please select your seats again.")
}

// ConfirmResult is returned by a successful payment.
type ConfirmResult struct {
	BookingID int64 `json:"bookingId"`
	// AlreadyConfirmed is true when this hold had been confirmed before (duplicate submit).
	AlreadyConfirmed bool `json:"alreadyConfirmed"`
}

// Confirm turns the user's hold into a booking. It is idempotent: submitting the same hold
// twice (double click, retry after a network error, two tabs) returns the same booking.
//
// Steps, inside one transaction, following the global lock order
// (show_seats -> holds -> hold_seats/bookings):
//  1. unlocked read of the hold to learn its show and seats (and check ownership);
//  2. lock those show_seats rows in seat_id order;
//  3. lock the hold row and re-validate: owner, status, not expired (using clock_timestamp());
//  4. insert booking + booking_seats (bookings.hold_id UNIQUE and the booking_seats partial
//     unique index are the database-level safeguards), record the payment, release the hold.
func (s *Store) Confirm(ctx context.Context, userID int64, holdID string) (*ConfirmResult, error) {
	var res ConfirmResult
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var showID int64
		err := tx.QueryRow(ctx, `SELECT show_id FROM holds WHERE id = $1 AND user_id = $2`, holdID, userID).Scan(&showID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errHoldNotFound
		}
		if err != nil {
			return err
		}
		seatIDs, err := holdSeatIDs(ctx, tx, holdID)
		if err != nil {
			return err
		}
		if _, err := seats.LockShowSeats(ctx, tx, showID, seatIDs); err != nil {
			return err
		}

		var status string
		var live bool
		if err := tx.QueryRow(ctx, `
			SELECT status, expires_at > clock_timestamp() FROM holds WHERE id = $1 FOR UPDATE`,
			holdID).Scan(&status, &live); err != nil {
			return err
		}
		switch {
		case status == "CONFIRMED":
			res.AlreadyConfirmed = true
			return tx.QueryRow(ctx, `SELECT id FROM bookings WHERE hold_id = $1`, holdID).Scan(&res.BookingID)
		case status == "RELEASED":
			return apperr.New(apperr.Gone, "HOLD_RELEASED", "This hold was released. Please select your seats again.")
		case !live:
			return holdExpired()
		}

		// Re-read under lock. While the hold is live nobody else may delete its seats, so this
		// should equal what we locked; anything else means the hold is no longer usable.
		lockedSeats := seatIDs
		if seatIDs, err = holdSeatIDs(ctx, tx, holdID); err != nil {
			return err
		}
		if len(seatIDs) == 0 || !slices.Equal(seatIDs, lockedSeats) {
			return holdExpired()
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO bookings (user_id, show_id, hold_id, status, total_paise)
			SELECT $1, $2, $3, 'CONFIRMED', price_paise * $4 FROM shows WHERE id = $2
			RETURNING id`, userID, showID, holdID, len(seatIDs)).Scan(&res.BookingID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO booking_seats (booking_id, show_id, seat_id)
			SELECT $1, $2, unnest($3::int[])`, res.BookingID, showID, seatIDs)
		if database.IsPgError(err, database.UniqueViolation) {
			// The final safeguard fired: some seat already has an active booking.
			return apperr.New(apperr.Conflict, "SEATS_ALREADY_BOOKED", "One or more seats have already been booked.")
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO payment_attempts (hold_id, user_id, outcome, amount_paise)
			SELECT $1, $2, 'SUCCESS', total_paise FROM bookings WHERE id = $3`, holdID, userID, res.BookingID); err != nil {
			return err
		}
		// Release the hold: the seats are now protected by booking_seats instead.
		if _, err := tx.Exec(ctx, `DELETE FROM hold_seats WHERE hold_id = $1`, holdID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE holds SET status = 'CONFIRMED' WHERE id = $1`, holdID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// RecordFailure handles a failed (fake) payment: it records the attempt and creates no
// booking. The hold is left untouched, so the user can retry until it expires.
func (s *Store) RecordFailure(ctx context.Context, userID int64, holdID string) (time.Time, error) {
	var expiresAt time.Time
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var status string
		var live bool
		var amount int
		err := tx.QueryRow(ctx, `
			SELECT h.status, h.expires_at, h.expires_at > clock_timestamp(),
			       s.price_paise * (SELECT count(*) FROM hold_seats hs WHERE hs.hold_id = h.id)
			FROM holds h JOIN shows s ON s.id = h.show_id
			WHERE h.id = $1 AND h.user_id = $2`, holdID, userID).Scan(&status, &expiresAt, &live, &amount)
		if errors.Is(err, pgx.ErrNoRows) {
			return errHoldNotFound
		}
		if err != nil {
			return err
		}
		switch {
		case status == "CONFIRMED":
			return apperr.New(apperr.Conflict, "HOLD_ALREADY_CONFIRMED", "This booking has already been paid for.")
		case status == "RELEASED":
			return apperr.New(apperr.Gone, "HOLD_RELEASED", "This hold was released. Please select your seats again.")
		case !live:
			return holdExpired()
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO payment_attempts (hold_id, user_id, outcome, amount_paise)
			VALUES ($1, $2, 'FAILURE', $3)`, holdID, userID, amount)
		return err
	})
	return expiresAt, err
}

// Cancel cancels the user's own booking if the show starts more than CancellationCutoff from
// now, releasing its seats in the same transaction (lock order: show_seats -> bookings).
func (s *Store) Cancel(ctx context.Context, userID, bookingID int64) error {
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var showID int64
		err := tx.QueryRow(ctx, `SELECT show_id FROM bookings WHERE id = $1 AND user_id = $2`, bookingID, userID).Scan(&showID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.New(apperr.NotFound, "BOOKING_NOT_FOUND", "Booking not found.")
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT seat_id FROM booking_seats WHERE booking_id = $1 ORDER BY seat_id`, bookingID)
		if err != nil {
			return err
		}
		seatIDs, err := pgx.CollectRows(rows, pgx.RowTo[int])
		if err != nil {
			return err
		}
		if _, err := seats.LockShowSeats(ctx, tx, showID, seatIDs); err != nil {
			return err
		}
		var status string
		var allowed bool
		if err := tx.QueryRow(ctx, `
			SELECT b.status, s.start_time - make_interval(hours => 2) > clock_timestamp()
			FROM bookings b JOIN shows s ON s.id = b.show_id
			WHERE b.id = $1 FOR UPDATE OF b`, bookingID).Scan(&status, &allowed); err != nil {
			return err
		}
		if status == "CANCELLED" {
			return apperr.New(apperr.Conflict, "ALREADY_CANCELLED", "This booking is already cancelled.")
		}
		if !allowed {
			return apperr.New(apperr.Conflict, "CANCELLATION_CLOSED",
				"Bookings can only be cancelled until 2 hours before the show starts.")
		}
		if _, err := tx.Exec(ctx, `UPDATE booking_seats SET active = false WHERE booking_id = $1`, bookingID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE bookings SET status = 'CANCELLED', cancelled_at = now() WHERE id = $1`, bookingID)
		return err
	})
}

func holdSeatIDs(ctx context.Context, tx pgx.Tx, holdID string) ([]int, error) {
	rows, err := tx.Query(ctx, `SELECT seat_id FROM hold_seats WHERE hold_id = $1 ORDER BY seat_id`, holdID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int])
}
