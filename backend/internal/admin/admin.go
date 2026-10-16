// Package admin contains the admin-only endpoints. Every route here is wrapped in
// auth.RequireAdmin by the router, so customers get 403 and anonymous users 401.
package admin

import (
	"context"
	"math"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"yozigo/internal/apperr"
	"yozigo/internal/bookings"
	"yozigo/internal/httpx"
	"yozigo/internal/movies"
	"yozigo/internal/seats"
	"yozigo/internal/shows"
)

type Handler struct {
	pool     *pgxpool.Pool
	movies   *movies.Store
	shows    *shows.Store
	seats    *seats.Store
	bookings *bookings.Store
	loc      *time.Location
}

func NewHandler(pool *pgxpool.Pool, m *movies.Store, s *shows.Store, st *seats.Store, b *bookings.Store, loc *time.Location) *Handler {
	return &Handler{pool: pool, movies: m, shows: s, seats: st, bookings: b, loc: loc}
}

// ---- Movies ----------------------------------------------------------------------------

// POST /api/admin/movies
func (h *Handler) CreateMovie(w http.ResponseWriter, r *http.Request) {
	var in movies.Input
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := in.Normalize(); err != nil {
		httpx.Error(w, r, err)
		return
	}
	m, err := h.movies.Create(r.Context(), in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"movie": m})
}

// PUT /api/admin/movies/{id}
func (h *Handler) UpdateMovie(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var in movies.Input
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := in.Normalize(); err != nil {
		httpx.Error(w, r, err)
		return
	}
	m, err := h.movies.Update(r.Context(), id, in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"movie": m})
}

// DELETE /api/admin/movies/{id}
func (h *Handler) DeleteMovie(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.movies.Delete(r.Context(), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Shows -----------------------------------------------------------------------------

// ShowStats is a show plus admin metrics.
type ShowStats struct {
	shows.Show
	OccupancyPercent float64 `json:"occupancyPercent"`
	RevenuePaise     int     `json:"revenuePaise"`
	BookingCount     int     `json:"bookingCount"`
}

// revenueByShow sums CONFIRMED bookings only (cancelled bookings are not revenue).
func (h *Handler) revenueByShow(ctx context.Context, showID *int64) (map[int64][2]int, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT show_id, COALESCE(sum(total_paise), 0), count(*)
		FROM bookings
		WHERE status = 'CONFIRMED' AND ($1::bigint IS NULL OR show_id = $1)
		GROUP BY show_id`, showID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// show id -> [revenue in paise, number of confirmed bookings]
	out := map[int64][2]int{}
	for rows.Next() {
		var id int64
		var revenue, count int
		if err := rows.Scan(&id, &revenue, &count); err != nil {
			return nil, err
		}
		out[id] = [2]int{revenue, count}
	}
	return out, rows.Err()
}

func withStats(s shows.Show, rev [2]int) ShowStats {
	occ := 0.0
	if s.TotalSeats > 0 {
		occ = math.Round(float64(s.BookedSeats)*1000/float64(s.TotalSeats)) / 10
	}
	return ShowStats{Show: s, OccupancyPercent: occ, RevenuePaise: rev[0], BookingCount: rev[1]}
}

// GET /api/admin/shows — all shows (past and upcoming) with occupancy and revenue.
func (h *Handler) ListShows(w http.ResponseWriter, r *http.Request) {
	all, err := h.shows.All(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	rev, err := h.revenueByShow(r.Context(), nil)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]ShowStats, len(all))
	for i, s := range all {
		out[i] = withStats(s, rev[s.ID])
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"shows": out})
}

// GET /api/admin/shows/{id} — stats, bookings (with customer names) and the seat map.
func (h *Handler) GetShow(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	s, err := h.shows.Get(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	rev, err := h.revenueByShow(r.Context(), &id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	list, err := h.bookings.ListForShow(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	seatMap, err := h.seats.SeatMap(r.Context(), id, 0)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"show": withStats(*s, rev[id]), "bookings": list, "seats": seatMap,
	})
}

type createShowRequest struct {
	MovieID    int64  `json:"movieId"`
	Date       string `json:"date"`      // YYYY-MM-DD, in the cinema's time zone
	StartTime  string `json:"startTime"` // HH:MM (24h), in the cinema's time zone
	PricePaise int    `json:"pricePaise"`
}

var (
	dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	timeRe = regexp.MustCompile(`^\d{2}:\d{2}$`)
)

// POST /api/admin/shows
//
// Date and time are interpreted in the cinema's configured time zone (CINEMA_TIMEZONE) and
// converted to an absolute instant, so overlap checks compare real instants regardless of
// the server's or browser's time zone (and stay correct across DST changes).
func (h *Handler) CreateShow(w http.ResponseWriter, r *http.Request) {
	var req createShowRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	fields := map[string]string{}
	if req.MovieID <= 0 {
		fields["movieId"] = "Choose a movie."
	}
	var start time.Time
	if !dateRe.MatchString(req.Date) || !timeRe.MatchString(req.StartTime) {
		fields["startTime"] = "Enter a valid date (YYYY-MM-DD) and time (HH:MM)."
	} else {
		var err error
		start, err = time.ParseInLocation("2006-01-02 15:04", req.Date+" "+req.StartTime, h.loc)
		if err != nil {
			fields["startTime"] = "Enter a valid date (YYYY-MM-DD) and time (HH:MM)."
		} else if !start.After(time.Now()) {
			fields["startTime"] = "Shows must be scheduled in the future."
		}
	}
	if req.PricePaise < 100 || req.PricePaise > 10_000_000 {
		fields["pricePaise"] = "Price must be between ₹1 and ₹1,00,000."
	}
	if len(fields) > 0 {
		httpx.Error(w, r, apperr.New(apperr.Invalid, "VALIDATION_FAILED", "Please fix the highlighted fields.").With("fields", fields))
		return
	}
	s, err := h.shows.Create(r.Context(), req.MovieID, start, req.PricePaise)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"show": withStats(*s, [2]int{})})
}

// DELETE /api/admin/shows/{id} — rejected with 409 if the show has bookings.
func (h *Handler) DeleteShow(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.shows.Delete(r.Context(), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
