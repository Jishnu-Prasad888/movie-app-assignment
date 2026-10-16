package auth

import (
	"context"
	"log/slog"
	"net/http"

	"yozigo/internal/apperr"
	"yozigo/internal/httpx"
)

const SessionCookieName = "yozigo_session"

type ctxKey struct{}

// CurrentUser returns the authenticated user for this request, or nil.
func CurrentUser(ctx context.Context) *User {
	u, _ := ctx.Value(ctxKey{}).(*User)
	return u
}

func withUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// Middleware resolves the session cookie (if any) into a *User on the request context.
// It never rejects a request by itself; RequireAuth / RequireAdmin do that.
type Middleware struct {
	store *Store
}

func NewMiddleware(store *Store) *Middleware { return &Middleware{store: store} }

func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(SessionCookieName); err == nil && c.Value != "" {
			u, err := m.store.UserBySession(r.Context(), c.Value)
			if err != nil {
				slog.Error("session lookup failed", "err", err)
				httpx.Error(w, r, err)
				return
			}
			if u != nil {
				r = r.WithContext(withUser(r.Context(), u))
			}
		}
		next.ServeHTTP(w, r)
	})
}

var errUnauthenticated = apperr.New(apperr.Unauthorized, "UNAUTHENTICATED", "Please log in to continue.")

// RequireAuth rejects requests without a valid session with 401.
func RequireAuth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if CurrentUser(r.Context()) == nil {
			httpx.Error(w, r, errUnauthenticated)
			return
		}
		next(w, r)
	})
}

// RequireAdmin rejects anonymous requests with 401 and non-admin users with 403.
func RequireAdmin(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := CurrentUser(r.Context())
		if u == nil {
			httpx.Error(w, r, errUnauthenticated)
			return
		}
		if !u.IsAdmin() {
			httpx.Error(w, r, apperr.New(apperr.Forbidden, "FORBIDDEN", "Admin access required."))
			return
		}
		next(w, r)
	})
}
