package movies

import (
	"net/http"

	"yozigo/internal/httpx"
	"yozigo/internal/shows"
)

// Handler serves the public (read-only) movie endpoints. Admin writes live in package admin.
type Handler struct {
	movies *Store
	shows  *shows.Store
}

func NewHandler(movies *Store, shows *shows.Store) *Handler {
	return &Handler{movies: movies, shows: shows}
}

// GET /api/movies
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.movies.List(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"movies": list})
}

// GET /api/movies/{id} — the movie plus its upcoming shows with live remaining-seat counts.
// Shows that have already started are excluded by the query.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	m, err := h.movies.Get(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	upcoming, err := h.shows.UpcomingForMovie(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"movie": m, "shows": upcoming})
}
