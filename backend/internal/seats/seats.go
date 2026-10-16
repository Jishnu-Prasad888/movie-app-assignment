// Package seats implements the seat map and seat holds.
//
// # Concurrency design (the important part)
//
// Every (show, seat) pair has a row in show_seats. Any transaction that changes the state of
// seats — acquiring a hold, releasing a hold, confirming a booking, cancelling a booking —
// first locks the relevant show_seats rows with SELECT ... FOR UPDATE, always in ascending
// seat_id order. Consequences:
//
//   - Two transactions touching the same seat are serialised: the second blocks until the first
//     commits or rolls back, and then (READ COMMITTED) its subsequent queries see the first one's
//     committed result. So "check the seat is free, then hold it" cannot interleave.
//   - Locking in one global order (seat_id ascending) means two transactions wanting
//     overlapping seat sets cannot each hold a lock the other needs, avoiding deadlocks.
//
// Global lock order used throughout the code base:
//
//	users row (hold requests only) -> show_seats rows (seat_id ASC) -> holds row -> hold_seats / bookings / booking_seats
//
// Database constraints back all of this up: hold_seats UNIQUE(show_id, seat_id) and the partial
// unique index on booking_seats(show_id, seat_id) WHERE active make double holds/bookings
// impossible even if a code path forgot to lock.
//
// Hold expiry is evaluated at read time (expires_at > now()), so an expired hold stops blocking
// a seat the instant it expires; the background cleanup only deletes stale rows.
package seats

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"yozigo/internal/apperr"
	"yozigo/internal/database"
	"yozigo/internal/shows"
)

const (
	MaxSeatsPerHold = 6

	StatusAvailable = "available"
	StatusHeld      = "held"        // held by another user
	StatusHeldByYou = "held_by_you" // held by the requesting user
	StatusBooked    = "booked"
)

type Seat struct {
	ID     int    `json:"id"`
	Label  string `json:"label"`
	Row    string `json:"row"`
	Number int    `json:"number"`
	Status string `json:"status"`
}

type SeatRef struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
}

// Hold is a user's temporary reservation of seats for one show.
type Hold struct {
	ID         string      `json:"id"`
	ShowID     int64       `json:"showId"`
	Status     string      `json:"status"` // ACTIVE, EXPIRED, CONFIRMED, RELEASED
	ExpiresAt  time.Time   `json:"expiresAt"`
	Seats      []SeatRef   `json:"seats"`
	PricePaise int         `json:"pricePaise"`
	TotalPaise int         `json:"totalPaise"`
	BookingID  *int64      `json:"bookingId"` // set once confirmed
	Show       *shows.Show `json:"show,omitempty"`
}

type Store struct {
	pool         *pgxpool.Pool
	shows        *shows.Store
	holdDuration time.Duration
}

func NewStore(pool *pgxpool.Pool, showStore *shows.Store, holdDuration time.Duration) *Store {
	return &Store{pool: pool, shows: showStore, holdDuration: holdDuration}
}

func SeatLabel(id int) string {
	return fmt.Sprintf("%c%d", 'A'+(id-1)/10, (id-1)%10+1)
}

func labels(ids []int) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = SeatLabel(id)
	}
	return out
}

// ValidateSeatSelection enforces 1..6 distinct, existing seat ids and returns them sorted.
func ValidateSeatSelection(ids []int) ([]int, error) {
	if len(ids) == 0 {
		return nil, apperr.New(apperr.Invalid, "NO_SEATS", "Select at least one seat.")
	}
	if len(ids) > MaxSeatsPerHold {
		return nil, apperr.New(apperr.Invalid, "TOO_MANY_SEATS",
			fmt.Sprintf("You can select at most %d seats per booking.", MaxSeatsPerHold))
	}
	sorted := slices.Clone(ids)
	slices.Sort(sorted)
	for i, id := range sorted {
		if id < 1 || id > 80 {
			return nil, apperr.New(apperr.Invalid, "INVALID_SEAT", "One or more seats do not exist.")
		}
		if i > 0 && sorted[i-1] == id {
			return nil, apperr.New(apperr.Invalid, "DUPLICATE_SEAT", "Each seat can only be selected once.")
		}
	}
	return sorted, nil
}

// LockShowSeats locks the show_seats rows for the given seats in ascending seat_id order
// (the global lock order) and returns the ids that exist. Must be called inside a transaction.
func LockShowSeats(ctx context.Context, tx pgx.Tx, showID int64, seatIDs []int) ([]int, error) {
	if len(seatIDs) == 0 {
		return nil, nil
	}
	// The sort happens below the LockRows step in the query plan, so rows are locked in
	// seat_id order.
	rows, err := tx.Query(ctx, `
		SELECT seat_id FROM show_seats
		WHERE show_id = $1 AND seat_id = ANY($2)
		ORDER BY seat_id
		FOR UPDATE`, showID, seatIDs)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int])
}

// SeatMap returns all 80 seats of a show with their current state. viewerID (0 for anonymous)
// is used only to distinguish the viewer's own holds.
func (s *Store) SeatMap(ctx context.Context, showID, viewerID int64) ([]Seat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT se.id, se.row_label, se.seat_number,
		       CASE WHEN bs.seat_id IS NOT NULL THEN 'booked'
		            WHEN h.id IS NOT NULL AND h.user_id = $2 THEN 'held_by_you'
		            WHEN h.id IS NOT NULL THEN 'held'
		            ELSE 'available' END
		FROM show_seats ss
		JOIN seats se ON se.id = ss.seat_id
		LEFT JOIN booking_seats bs ON bs.show_id = ss.show_id AND bs.seat_id = ss.seat_id AND bs.active
		LEFT JOIN hold_seats hs ON hs.show_id = ss.show_id AND hs.seat_id = ss.seat_id
		LEFT JOIN holds h ON h.id = hs.hold_id AND h.status = 'ACTIVE' AND h.expires_at > now()
		WHERE ss.show_id = $1
		ORDER BY se.id`, showID, viewerID)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Seat, error) {
		var st Seat
		err := row.Scan(&st.ID, &st.Row, &st.Number, &st.Status)
		st.Label = fmt.Sprintf("%s%d", st.Row, st.Number)
		return st, err
	})
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, apperr.New(apperr.NotFound, "SHOW_NOT_FOUND", "Show not found.")
	}
	return list, nil
}

// AcquireHold atomically holds all requested seats for userID, or none of them.
//
// Any previous active hold of the same user on the same show is released in the same
// transaction (one hold per user per show, so the 6-seat limit cannot be bypassed by holding
// repeatedly).
func (s *Store) AcquireHold(ctx context.Context, userID, showID int64, requested []int) (*Hold, error) {
	seatIDs, err := ValidateSeatSelection(requested)
	if err != nil {
		return nil, err
	}

	var holdID string
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		// 1. Serialise this user's own hold requests (e.g. two browser tabs) so the
		//    "release my previous hold" step below cannot miss a hold created concurrently.
		//    FOR NO KEY UPDATE does not block the FK checks other transactions do on users.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM users WHERE id = $1 FOR NO KEY UPDATE`, userID); err != nil {
			return err
		}

		// 2. The show must exist and not have started.
		var started bool
		err := tx.QueryRow(ctx, `SELECT start_time <= clock_timestamp() FROM shows WHERE id = $1`, showID).Scan(&started)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.New(apperr.NotFound, "SHOW_NOT_FOUND", "Show not found.")
		}
		if err != nil {
			return err
		}
		if started {
			return apperr.New(apperr.Gone, "SHOW_STARTED", "This show has already started.")
		}

		// 3. Lock every seat whose state we may change: the requested seats plus the seats of
		//    the user's current hold on this show (which we are about to release).
		rows, err := tx.Query(ctx, `
			SELECT hs.seat_id FROM holds h JOIN hold_seats hs ON hs.hold_id = h.id
			WHERE h.user_id = $1 AND h.show_id = $2 AND h.status = 'ACTIVE'`, userID, showID)
		if err != nil {
			return err
		}
		previous, err := pgx.CollectRows(rows, pgx.RowTo[int])
		if err != nil {
			return err
		}
		toLock := slices.Compact(slices.Sorted(slices.Values(append(slices.Clone(seatIDs), previous...))))
		locked, err := LockShowSeats(ctx, tx, showID, toLock)
		if err != nil {
			return err
		}
		if len(locked) != len(toLock) {
			return apperr.New(apperr.Invalid, "INVALID_SEAT", "One or more seats do not exist.")
		}

		// 4. With the locks held, nobody else can change these seats. Check for conflicts:
		//    an active booking, or a live hold by a different user. clock_timestamp() (not
		//    now(), which is frozen at transaction start) because we may have waited for locks.
		rows, err = tx.Query(ctx, `
			SELECT seat_id FROM booking_seats
			WHERE show_id = $1 AND seat_id = ANY($2) AND active
			UNION
			SELECT hs.seat_id FROM hold_seats hs JOIN holds h ON h.id = hs.hold_id
			WHERE hs.show_id = $1 AND hs.seat_id = ANY($2)
			  AND h.user_id <> $3 AND h.status = 'ACTIVE' AND h.expires_at > clock_timestamp()
			ORDER BY 1`, showID, seatIDs, userID)
		if err != nil {
			return err
		}
		conflicts, err := pgx.CollectRows(rows, pgx.RowTo[int])
		if err != nil {
			return err
		}
		if len(conflicts) > 0 {
			// All-or-nothing: returning an error rolls back the whole transaction.
			return apperr.New(apperr.Conflict, "SEATS_UNAVAILABLE",
				"Some of the selected seats were just taken by someone else. Please choose again.").
				With("conflictingSeatIds", conflicts).
				With("conflictingSeats", labels(conflicts))
		}

		// 5. Release the user's previous hold on this show (holds row before hold_seats rows).
		if _, err := tx.Exec(ctx, `
			WITH released AS (
				UPDATE holds SET status = 'RELEASED'
				WHERE user_id = $1 AND show_id = $2 AND status = 'ACTIVE'
				RETURNING id
			)
			DELETE FROM hold_seats WHERE hold_id IN (SELECT id FROM released)`, userID, showID); err != nil {
			return err
		}

		// 6. Any hold_seats rows still on the requested seats belong to expired holds; remove
		//    them so the UNIQUE(show_id, seat_id) constraint lets us insert ours.
		if _, err := tx.Exec(ctx, `DELETE FROM hold_seats WHERE show_id = $1 AND seat_id = ANY($2)`, showID, seatIDs); err != nil {
			return err
		}

		// 7. Create the hold.
		if err := tx.QueryRow(ctx, `
			INSERT INTO holds (user_id, show_id, expires_at)
			VALUES ($1, $2, clock_timestamp() + $3::interval)
			RETURNING id::text`, userID, showID, s.holdDuration.String()).Scan(&holdID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO hold_seats (hold_id, show_id, seat_id)
			SELECT $1, $2, unnest($3::int[])`, holdID, showID, seatIDs)
		if database.IsPgError(err, database.UniqueViolation) {
			// Unreachable while the locking above is correct; the constraint is the backstop.
			return apperr.New(apperr.Conflict, "SEATS_UNAVAILABLE", "Some of the selected seats are no longer available.")
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GetHold(ctx, userID, holdID)
}

func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'):
		default:
			return false
		}
	}
	return true
}

var errHoldNotFound = apperr.New(apperr.NotFound, "HOLD_NOT_FOUND", "Hold not found.")

// GetHold returns a hold owned by userID. Holds of other users are reported as not found.
func (s *Store) GetHold(ctx context.Context, userID int64, holdID string) (*Hold, error) {
	if !validUUID(holdID) {
		return nil, errHoldNotFound
	}
	var h Hold
	var expired bool
	err := s.pool.QueryRow(ctx, `
		SELECT h.id::text, h.show_id, h.status, h.expires_at, h.expires_at <= now(), s.price_paise,
		       (SELECT b.id FROM bookings b WHERE b.hold_id = h.id)
		FROM holds h JOIN shows s ON s.id = h.show_id
		WHERE h.id = $1 AND h.user_id = $2`, holdID, userID).
		Scan(&h.ID, &h.ShowID, &h.Status, &h.ExpiresAt, &expired, &h.PricePaise, &h.BookingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errHoldNotFound
	}
	if err != nil {
		return nil, err
	}
	if h.Status == "ACTIVE" && expired {
		h.Status = "EXPIRED"
	}
	rows, err := s.pool.Query(ctx, `SELECT seat_id FROM hold_seats WHERE hold_id = $1 ORDER BY seat_id`, holdID)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return nil, err
	}
	h.Seats = make([]SeatRef, len(ids))
	for i, id := range ids {
		h.Seats[i] = SeatRef{ID: id, Label: SeatLabel(id)}
	}
	h.TotalPaise = h.PricePaise * len(ids)
	if h.Show, err = s.shows.Get(ctx, h.ShowID); err != nil {
		return nil, err
	}
	return &h, nil
}

// ActiveHoldID returns the id of the user's live hold on a show, if any.
func (s *Store) ActiveHoldID(ctx context.Context, userID, showID int64) (*string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text FROM holds
		WHERE user_id = $1 AND show_id = $2 AND status = 'ACTIVE' AND expires_at > now()
		ORDER BY created_at DESC LIMIT 1`, userID, showID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// ReleaseHold lets a user give up their own hold early (e.g. "change seats").
func (s *Store) ReleaseHold(ctx context.Context, userID int64, holdID string) error {
	if !validUUID(holdID) {
		return errHoldNotFound
	}
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var showID int64
		err := tx.QueryRow(ctx, `SELECT show_id FROM holds WHERE id = $1 AND user_id = $2`, holdID, userID).Scan(&showID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errHoldNotFound
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT seat_id FROM hold_seats WHERE hold_id = $1 ORDER BY seat_id`, holdID)
		if err != nil {
			return err
		}
		seatIDs, err := pgx.CollectRows(rows, pgx.RowTo[int])
		if err != nil {
			return err
		}
		if _, err := LockShowSeats(ctx, tx, showID, seatIDs); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE holds SET status = 'RELEASED' WHERE id = $1 AND status = 'ACTIVE'`, holdID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil // already confirmed or released: nothing to do
		}
		_, err = tx.Exec(ctx, `DELETE FROM hold_seats WHERE hold_id = $1`, holdID)
		return err
	})
}

// PurgeExpiredHoldSeats deletes hold_seats rows of holds that expired more than a minute ago.
// Purely housekeeping: correctness never depends on it, because every query already treats
// expired holds as inactive. SKIP LOCKED means cleanup never waits on (or deadlocks with) a
// booking transaction; rows that are busy are simply picked up on the next run.
func (s *Store) PurgeExpiredHoldSeats(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM hold_seats
		WHERE (hold_id, seat_id) IN (
			SELECT hs.hold_id, hs.seat_id
			FROM hold_seats hs JOIN holds h ON h.id = hs.hold_id
			WHERE h.status = 'ACTIVE' AND h.expires_at < now() - interval '1 minute'
			FOR UPDATE OF hs SKIP LOCKED
		)`)
	return tag.RowsAffected(), err
}
