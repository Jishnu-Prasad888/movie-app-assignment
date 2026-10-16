package seats

import (
	"net/http"
	"time"

	"yozigo/internal/auth"
	"yozigo/internal/httpx"
	"yozigo/internal/shows"
)

type Handler struct {
	seats *Store
	shows *shows.Store
}

func NewHandler(seats *Store, shows *shows.Store) *Handler {
	return &Handler{seats: seats, shows: shows}
}

// GET /api/shows/{id}/seats — show details, the 80-seat map with live states, and the
// viewer's own active hold on this show (so the UI can resume checkout).
func (h *Handler) SeatMap(w http.ResponseWriter, r *http.Request) {
	showID, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	show, err := h.shows.Get(r.Context(), showID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var viewerID int64
	var activeHoldID *string
	if u := auth.CurrentUser(r.Context()); u != nil {
		viewerID = u.ID
		if activeHoldID, err = h.seats.ActiveHoldID(r.Context(), u.ID, showID); err != nil {
			httpx.Error(w, r, err)
			return
		}
	}
	seatList, err := h.seats.SeatMap(r.Context(), showID, viewerID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"show":         show,
		"seats":        seatList,
		"activeHoldId": activeHoldID,
		"maxSeats":     MaxSeatsPerHold,
		"serverTime":   time.Now().UTC(),
	})
}

type holdRequest struct {
	SeatIDs []int `json:"seatIds"`
}

// POST /api/shows/{id}/holds  {"seatIds": [1, 2, 3]}
func (h *Handler) CreateHold(w http.ResponseWriter, r *http.Request) {
	showID, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req holdRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	// The user comes from the session, never from the request body.
	user := auth.CurrentUser(r.Context())
	hold, err := h.seats.AcquireHold(r.Context(), user.ID, showID, req.SeatIDs)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"hold": hold, "serverTime": time.Now().UTC()})
}

// GET /api/holds/{id}
func (h *Handler) GetHold(w http.ResponseWriter, r *http.Request) {
	hold, err := h.seats.GetHold(r.Context(), auth.CurrentUser(r.Context()).ID, r.PathValue("id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"hold": hold, "serverTime": time.Now().UTC()})
}

// DELETE /api/holds/{id}
func (h *Handler) ReleaseHold(w http.ResponseWriter, r *http.Request) {
	if err := h.seats.ReleaseHold(r.Context(), auth.CurrentUser(r.Context()).ID, r.PathValue("id")); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
