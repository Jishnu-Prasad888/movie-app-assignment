// Package shows contains show scheduling (with single-screen overlap validation) and
// availability queries.
package shows

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"yozigo/internal/apperr"
	"yozigo/internal/database"
)

// Show is a scheduled screening plus live availability counts computed from the database.
type Show struct {
	ID              int64     `json:"id"`
	MovieID         int64     `json:"movieId"`
	MovieTitle      string    `json:"movieTitle"`
	MovieLanguage   string    `json:"movieLanguage"`
	MovieRating     string    `json:"movieRating"`
	DurationMinutes int       `json:"durationMinutes"`
	StartTime       time.Time `json:"startTime"`
	EndTime         time.Time `json:"endTime"`
	PricePaise      int       `json:"pricePaise"`
	TotalSeats      int       `json:"totalSeats"`
	BookedSeats     int       `json:"bookedSeats"`
	HeldSeats       int       `json:"heldSeats"`
	RemainingSeats  int       `json:"remainingSeats"`
}

// showSelect returns shows with availability. A hold counts only while it is ACTIVE and
// not past expires_at, so expired holds free their seats immediately, cleanup or not.
const showSelect = `
	SELECT s.id, s.movie_id, m.title, m.language, m.rating, m.duration_minutes,
	       s.start_time, s.end_time, s.price_paise,
	       (SELECT count(*) FROM show_seats ss WHERE ss.show_id = s.id) AS total,
	       (SELECT count(*) FROM booking_seats bs WHERE bs.show_id = s.id AND bs.active) AS booked,
	       (SELECT count(*) FROM hold_seats hs JOIN holds h ON h.id = hs.hold_id
	         WHERE hs.show_id = s.id AND h.status = 'ACTIVE' AND h.expires_at > now()) AS held
	FROM shows s JOIN movies m ON m.id = s.movie_id`

func scanShow(row pgx.Row) (*Show, error) {
	var s Show
	err := row.Scan(&s.ID, &s.MovieID, &s.MovieTitle, &s.MovieLanguage, &s.MovieRating,
		&s.DurationMinutes, &s.StartTime, &s.EndTime, &s.PricePaise,
		&s.TotalSeats, &s.BookedSeats, &s.HeldSeats)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.New(apperr.NotFound, "SHOW_NOT_FOUND", "Show not found.")
	}
	if err != nil {
		return nil, err
	}
	s.RemainingSeats = s.TotalSeats - s.BookedSeats - s.HeldSeats
	return &s, nil
}

func collect(rows pgx.Rows, err error) ([]Show, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Show{}
	for rows.Next() {
		s, err := scanShow(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *s)
	}
	return list, rows.Err()
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// UpcomingForMovie lists shows of a movie that have not started yet, earliest first.
func (s *Store) UpcomingForMovie(ctx context.Context, movieID int64) ([]Show, error) {
	return collect(s.pool.Query(ctx, showSelect+`
		WHERE s.movie_id = $1 AND s.start_time > now()
		ORDER BY s.start_time`, movieID))
}

// All lists every show (past and future), used by the admin dashboard.
func (s *Store) All(ctx context.Context) ([]Show, error) {
	return collect(s.pool.Query(ctx, showSelect+` ORDER BY s.start_time`))
}

func (s *Store) Get(ctx context.Context, id int64) (*Show, error) {
	return scanShow(s.pool.QueryRow(ctx, showSelect+` WHERE s.id = $1`, id))
}

// Create schedules a show. end_time = start + the movie's duration. Overlap with any other
// show is checked twice:
//  1. an explicit query, to return a friendly message naming the conflicting show;
//  2. the shows_no_overlap exclusion constraint, which is race-proof (two concurrent inserts
//     cannot both pass) and therefore the real guarantee.
func (s *Store) Create(ctx context.Context, movieID int64, start time.Time, pricePaise int) (*Show, error) {
	var id int64
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var duration int
		// FOR SHARE: the movie's duration cannot change until this transaction finishes.
		err := tx.QueryRow(ctx, `SELECT duration_minutes FROM movies WHERE id = $1 FOR SHARE`, movieID).Scan(&duration)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.New(apperr.Invalid, "MOVIE_NOT_FOUND", "Movie not found.")
		}
		if err != nil {
			return err
		}
		end := start.Add(time.Duration(duration) * time.Minute)

		var conflictTitle string
		var conflictStart, conflictEnd time.Time
		err = tx.QueryRow(ctx, `
			SELECT m.title, s.start_time, s.end_time
			FROM shows s JOIN movies m ON m.id = s.movie_id
			WHERE tstzrange(s.start_time, s.end_time, '[)') && tstzrange($1, $2, '[)')
			ORDER BY s.start_time LIMIT 1`, start, end).Scan(&conflictTitle, &conflictStart, &conflictEnd)
		if err == nil {
			return overlapError().
				With("conflictingShow", map[string]any{"movieTitle": conflictTitle, "startTime": conflictStart, "endTime": conflictEnd})
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO shows (movie_id, start_time, end_time, price_paise)
			VALUES ($1, $2, $3, $4) RETURNING id`, movieID, start, end, pricePaise).Scan(&id)
		if database.IsPgError(err, database.ExclusionViolation) {
			return overlapError()
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func overlapError() *apperr.Error {
	return apperr.New(apperr.Conflict, "SHOW_OVERLAP", "This show overlaps with another show on the screen.")
}

// Delete removes a show only if it has no bookings (confirmed or cancelled — booking history
// is never destroyed). FK bookings.show_id ON DELETE RESTRICT enforces the same rule in the
// database, which also covers a booking committed concurrently with this delete.
func (s *Store) Delete(ctx context.Context, id int64) error {
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM shows WHERE id = $1 FOR UPDATE)`, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apperr.New(apperr.NotFound, "SHOW_NOT_FOUND", "Show not found.")
		}
		hasBookings := apperr.New(apperr.Conflict, "SHOW_HAS_BOOKINGS", "This show has bookings and cannot be deleted.")
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM bookings WHERE show_id = $1`, id).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return hasBookings
		}
		_, err := tx.Exec(ctx, `DELETE FROM shows WHERE id = $1`, id)
		if database.IsPgError(err, database.ForeignKeyViolation) {
			return hasBookings
		}
		return err
	})
}
