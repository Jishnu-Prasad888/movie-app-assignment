package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"yozigo/internal/apperr"
	"yozigo/internal/database"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const userColumns = `id, name, email, role, created_at`

func scanUser(row pgx.Row, extra ...any) (*User, error) {
	var u User
	dest := append([]any{&u.ID, &u.Name, &u.Email, &u.Role, &u.CreatedAt}, extra...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}
	return &u, nil
}

// CreateCustomer inserts a new user. The role is hard-coded to customer: signup can never
// create an admin, whatever the request contains.
func (s *Store) CreateCustomer(ctx context.Context, name, email, passwordHash string) (*User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash, role)
		VALUES ($1, $2, $3, 'customer')
		RETURNING `+userColumns, name, email, passwordHash))
	if database.IsPgError(err, database.UniqueViolation) {
		return nil, apperr.New(apperr.Conflict, "EMAIL_TAKEN", "An account with this email already exists.")
	}
	return u, err
}

// UserWithHashByEmail returns the user and password hash, or (nil, "", nil) if not found.
func (s *Store) UserWithHashByEmail(ctx context.Context, email string) (*User, string, error) {
	var hash string
	u, err := scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+`, password_hash FROM users WHERE email = $1`, email), &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", nil
	}
	return u, hash, err
}

func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	u, _, err := s.UserWithHashByEmail(ctx, email)
	return u, err
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// CreateSession issues a new random 256-bit session token for userID. Only its SHA-256 hash
// is stored; the plain token goes to the browser in an HttpOnly cookie.
func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (string, time.Time, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	var expires time.Time
	err := s.pool.QueryRow(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at)
		VALUES ($1, $2, now() + $3::interval)
		RETURNING expires_at`, hashToken(token), userID, ttl.String()).Scan(&expires)
	return token, expires, err
}

// UserBySession resolves a session token to its user, or nil if missing/expired. The role
// comes from the users table on every request, so role changes take effect immediately.
func (s *Store) UserBySession(ctx context.Context, token string) (*User, error) {
	u, err := scanUser(s.pool.QueryRow(ctx, `
		SELECT u.id, u.name, u.email, u.role, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, hashToken(token)))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return u, err
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hashToken(token))
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	return tag.RowsAffected(), err
}
