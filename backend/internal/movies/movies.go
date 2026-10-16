// Package movies contains the movie catalogue: data access, validation and public handlers.
package movies

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"yozigo/internal/apperr"
	"yozigo/internal/database"
)

type Movie struct {
	ID              int64     `json:"id"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	Language        string    `json:"language"`
	DurationMinutes int       `json:"durationMinutes"`
	Rating          string    `json:"rating"`
	PosterURL       *string   `json:"posterUrl"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// Input is the admin-supplied payload for creating or updating a movie.
type Input struct {
	Title           string  `json:"title"`
	Description     string  `json:"description"`
	Language        string  `json:"language"`
	DurationMinutes int     `json:"durationMinutes"`
	Rating          string  `json:"rating"`
	PosterURL       *string `json:"posterUrl"`
}

// Normalize trims fields and validates them, returning a field -> message map on failure.
func (in *Input) Normalize() error {
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.Language = strings.TrimSpace(in.Language)
	in.Rating = strings.ToUpper(strings.TrimSpace(in.Rating))
	if in.PosterURL != nil {
		p := strings.TrimSpace(*in.PosterURL)
		if p == "" {
			in.PosterURL = nil
		} else {
			in.PosterURL = &p
		}
	}

	fields := map[string]string{}
	if in.Title == "" || utf8.RuneCountInString(in.Title) > 200 {
		fields["title"] = "Title is required (max 200 characters)."
	}
	if utf8.RuneCountInString(in.Description) > 2000 {
		fields["description"] = "Description must be at most 2000 characters."
	}
	if in.Language == "" || utf8.RuneCountInString(in.Language) > 50 {
		fields["language"] = "Language is required (max 50 characters)."
	}
	if in.DurationMinutes < 1 || in.DurationMinutes > 600 {
		fields["durationMinutes"] = "Duration must be between 1 and 600 minutes."
	}
	if in.Rating != "U" && in.Rating != "UA" && in.Rating != "A" {
		fields["rating"] = "Rating must be U, UA or A."
	}
	if in.PosterURL != nil {
		u, err := url.Parse(*in.PosterURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(*in.PosterURL) > 1000 {
			fields["posterUrl"] = "Poster URL must be a valid http(s) URL."
		}
	}
	if len(fields) > 0 {
		return apperr.New(apperr.Invalid, "VALIDATION_FAILED", "Please fix the highlighted fields.").With("fields", fields)
	}
	return nil
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const movieColumns = `id, title, description, language, duration_minutes, rating, poster_url, created_at, updated_at`

func scanMovie(row pgx.Row) (*Movie, error) {
	var m Movie
	err := row.Scan(&m.ID, &m.Title, &m.Description, &m.Language, &m.DurationMinutes,
		&m.Rating, &m.PosterURL, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.New(apperr.NotFound, "MOVIE_NOT_FOUND", "Movie not found.")
	}
	return &m, err
}

func (s *Store) List(ctx context.Context) ([]Movie, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+movieColumns+` FROM movies ORDER BY title, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Movie{}
	for rows.Next() {
		m, err := scanMovie(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *m)
	}
	return list, rows.Err()
}

func (s *Store) Get(ctx context.Context, id int64) (*Movie, error) {
	return scanMovie(s.pool.QueryRow(ctx, `SELECT `+movieColumns+` FROM movies WHERE id = $1`, id))
}

func (s *Store) Create(ctx context.Context, in Input) (*Movie, error) {
	return scanMovie(s.pool.QueryRow(ctx, `
		INSERT INTO movies (title, description, language, duration_minutes, rating, poster_url)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+movieColumns,
		in.Title, in.Description, in.Language, in.DurationMinutes, in.Rating, in.PosterURL))
}

// Update edits a movie. If the duration changes, the end_time of its shows that have not
// started yet is recomputed in the same transaction; the shows_no_overlap exclusion
// constraint rejects the change if a longer movie would now collide with the next show.
func (s *Store) Update(ctx context.Context, id int64, in Input) (*Movie, error) {
	var out *Movie
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var oldDuration int
		err := tx.QueryRow(ctx, `SELECT duration_minutes FROM movies WHERE id = $1 FOR UPDATE`, id).Scan(&oldDuration)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.New(apperr.NotFound, "MOVIE_NOT_FOUND", "Movie not found.")
		}
		if err != nil {
			return err
		}
		out, err = scanMovie(tx.QueryRow(ctx, `
			UPDATE movies SET title = $2, description = $3, language = $4, duration_minutes = $5,
			       rating = $6, poster_url = $7, updated_at = now()
			WHERE id = $1
			RETURNING `+movieColumns,
			id, in.Title, in.Description, in.Language, in.DurationMinutes, in.Rating, in.PosterURL))
		if err != nil {
			return err
		}
		if oldDuration != in.DurationMinutes {
			_, err = tx.Exec(ctx, `
				UPDATE shows SET end_time = start_time + make_interval(mins => $2)
				WHERE movie_id = $1 AND start_time > now()`, id, in.DurationMinutes)
			if database.IsPgError(err, database.ExclusionViolation) {
				return apperr.New(apperr.Conflict, "SHOW_OVERLAP",
					"The new duration would make an upcoming show of this movie overlap with another show.")
			}
			return err
		}
		return nil
	})
	return out, err
}

// Delete removes a movie and its shows, but only if none of those shows has any booking.
// The FK bookings.show_id ON DELETE RESTRICT is the database-level safeguard for this rule.
func (s *Store) Delete(ctx context.Context, id int64) error {
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM movies WHERE id = $1 FOR UPDATE)`, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apperr.New(apperr.NotFound, "MOVIE_NOT_FOUND", "Movie not found.")
		}
		var hasBookings bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM bookings b JOIN shows s ON s.id = b.show_id WHERE s.movie_id = $1)`,
			id).Scan(&hasBookings); err != nil {
			return err
		}
		if hasBookings {
			return apperr.New(apperr.Conflict, "MOVIE_HAS_BOOKINGS", "This movie has shows with bookings and cannot be deleted.")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM shows WHERE movie_id = $1`, id); err != nil {
			if database.IsPgError(err, database.ForeignKeyViolation) {
				return apperr.New(apperr.Conflict, "MOVIE_HAS_BOOKINGS", "This movie has shows with bookings and cannot be deleted.")
			}
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM movies WHERE id = $1`, id)
		return err
	})
}
