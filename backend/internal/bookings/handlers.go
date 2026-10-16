package bookings

import (
	"net/http"

	"yozigo/internal/apperr"
	"yozigo/internal/auth"
	"yozigo/internal/httpx"
)

type Handler struct {
	store *Store
}

func NewHandler(store *Store) *Handler { return &Handler{store: store} }

type paymentRequest struct {
	// The fake gateway's result: "success" or "failure".
	Outcome string `json:"outcome"`
}

// POST /api/holds/{id}/payment  {"outcome": "success" | "failure"}
//
// Simulates the payment gateway callback. success -> booking confirmed (201, idempotent: a
// repeat returns 200 with the same booking). failure -> 402, no booking, hold kept.
func (h *Handler) Pay(w http.ResponseWriter, r *http.Request) {
	var req paymentRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	user := auth.CurrentUser(r.Context())
	holdID := r.PathValue("id")
	if len(holdID) != 36 {
		httpx.Error(w, r, errHoldNotFound)
		return
	}
	switch req.Outcome {
	case "success":
		res, err := h.store.Confirm(r.Context(), user.ID, holdID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		b, err := h.store.GetForUser(r.Context(), user.ID, res.BookingID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		status := http.StatusCreated
		if res.AlreadyConfirmed {
			status = http.StatusOK
		}
		httpx.JSON(w, status, map[string]any{"booking": b, "alreadyConfirmed": res.AlreadyConfirmed})
	case "failure":
		expiresAt, err := h.store.RecordFailure(r.Context(), user.ID, holdID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.Error(w, r, apperr.New(apperr.PaymentFailed, "PAYMENT_FAILED",
			"Payment failed. No money was taken and your seats are still held — you can try again.").
			With("holdExpiresAt", expiresAt))
	default:
		httpx.Error(w, r, apperr.New(apperr.Invalid, "INVALID_OUTCOME", `outcome must be "success" or "failure".`))
	}
}

// GET /api/bookings — the current user's bookings only.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListForUser(r.Context(), auth.CurrentUser(r.Context()).ID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"bookings": list})
}

// GET /api/bookings/{id} — 404 unless the booking belongs to the current user.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	b, err := h.store.GetForUser(r.Context(), auth.CurrentUser(r.Context()).ID, id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"booking": b})
}

// POST /api/bookings/{id}/cancel
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	user := auth.CurrentUser(r.Context())
	if err := h.store.Cancel(r.Context(), user.ID, id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	b, err := h.store.GetForUser(r.Context(), user.ID, id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"booking": b})
}
