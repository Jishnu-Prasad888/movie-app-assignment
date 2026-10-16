// Package server wires stores, handlers and middleware into the HTTP router.
package server

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"yozigo/internal/admin"
	"yozigo/internal/auth"
	"yozigo/internal/bookings"
	"yozigo/internal/config"
	"yozigo/internal/httpx"
	"yozigo/internal/middleware"
	"yozigo/internal/movies"
	"yozigo/internal/seats"
	"yozigo/internal/shows"
)

// Stores groups the data-access objects, so main can reuse them (e.g. for cleanup).
type Stores struct {
	Auth     *auth.Store
	Movies   *movies.Store
	Shows    *shows.Store
	Seats    *seats.Store
	Bookings *bookings.Store
}

func NewStores(pool *pgxpool.Pool, cfg config.Config) *Stores {
	showStore := shows.NewStore(pool)
	return &Stores{
		Auth:     auth.NewStore(pool),
		Movies:   movies.NewStore(pool),
		Shows:    showStore,
		Seats:    seats.NewStore(pool, showStore, cfg.HoldDuration),
		Bookings: bookings.NewStore(pool),
	}
}

// New builds the complete HTTP handler.
//
// Route protection is declared next to each route:
//   - public:          no wrapper
//   - authenticated:   auth.RequireAuth  (401 without a valid session)
//   - admin only:      auth.RequireAdmin (401 anonymous, 403 for customers)
func New(cfg config.Config, pool *pgxpool.Pool, st *Stores) http.Handler {
	authH := auth.NewHandler(st.Auth, cfg)
	movieH := movies.NewHandler(st.Movies, st.Shows)
	seatH := seats.NewHandler(st.Seats, st.Shows)
	bookingH := bookings.NewHandler(st.Bookings)
	adminH := admin.NewHandler(pool, st.Movies, st.Shows, st.Seats, st.Bookings, cfg.CinemaTimezone)

	user := auth.RequireAuth
	adminOnly := auth.RequireAdmin

	mux := http.NewServeMux()

	// Health & config
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			httpx.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]any{
			"demoLoginEnabled":    cfg.DemoLoginEnabled,
			"timezone":            cfg.CinemaTimezone.String(),
			"holdDurationSeconds": int(cfg.HoldDuration / time.Second),
			"maxSeatsPerBooking":  seats.MaxSeatsPerHold,
		})
	})

	// Auth
	mux.HandleFunc("POST /api/auth/signup", authH.Signup)
	mux.HandleFunc("POST /api/auth/login", authH.Login)
	mux.HandleFunc("POST /api/auth/demo-login", authH.DemoLogin)
	mux.HandleFunc("POST /api/auth/logout", authH.Logout)
	mux.HandleFunc("GET /api/auth/me", authH.Me)

	// Catalogue (public)
	mux.HandleFunc("GET /api/movies", movieH.List)
	mux.HandleFunc("GET /api/movies/{id}", movieH.Get)
	mux.HandleFunc("GET /api/shows/{id}/seats", seatH.SeatMap)

	// Booking flow (logged-in users)
	mux.Handle("POST /api/shows/{id}/holds", user(seatH.CreateHold))
	mux.Handle("GET /api/holds/{id}", user(seatH.GetHold))
	mux.Handle("DELETE /api/holds/{id}", user(seatH.ReleaseHold))
	mux.Handle("POST /api/holds/{id}/payment", user(bookingH.Pay))
	mux.Handle("GET /api/bookings", user(bookingH.List))
	mux.Handle("GET /api/bookings/{id}", user(bookingH.Get))
	mux.Handle("POST /api/bookings/{id}/cancel", user(bookingH.Cancel))

	// Admin
	mux.Handle("POST /api/admin/movies", adminOnly(adminH.CreateMovie))
	mux.Handle("PUT /api/admin/movies/{id}", adminOnly(adminH.UpdateMovie))
	mux.Handle("DELETE /api/admin/movies/{id}", adminOnly(adminH.DeleteMovie))
	mux.Handle("GET /api/admin/shows", adminOnly(adminH.ListShows))
	mux.Handle("POST /api/admin/shows", adminOnly(adminH.CreateShow))
	mux.Handle("GET /api/admin/shows/{id}", adminOnly(adminH.GetShow))
	mux.Handle("DELETE /api/admin/shows/{id}", adminOnly(adminH.DeleteShow))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusNotFound, map[string]string{"error": "Not found.", "code": "NOT_FOUND"})
	})

	authMW := auth.NewMiddleware(st.Auth)
	var h http.Handler = mux
	h = authMW.Authenticate(h)
	h = middleware.CSRFHeader(h)
	h = middleware.SecurityHeaders(h)
	h = middleware.Recoverer(h)
	h = middleware.Logger(h)
	return h
}
