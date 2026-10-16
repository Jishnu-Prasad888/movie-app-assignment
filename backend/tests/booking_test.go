package tests

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// Required test 3: an expired hold stops blocking the seat immediately, without any cleanup job.
func TestExpiredHoldBecomesAvailable(t *testing.T) {
	e := setup(t)
	a, b := e.customer("Alice"), e.customer("Bob")
	showID := e.defaultShow()

	holdA := e.hold(a, showID, 15, 16)
	if s := e.seatStatus(b, showID, 15); s != "held" {
		t.Fatalf("seat status for B = %q, want held", s)
	}
	// B cannot take it while A's hold is live.
	r := b.post(fmt.Sprintf("/api/shows/%d/holds", showID), map[string]any{"seatIds": []int{15}})
	expectStatus(t, r, http.StatusConflict)
	if got := r.Body["conflictingSeats"].([]any); len(got) != 1 || got[0] != "B5" {
		t.Fatalf("conflictingSeats = %v, want [B5]", got)
	}

	e.expireHold(holdA) // the cleanup loop is NOT running in tests

	if s := e.seatStatus(b, showID, 15); s != "available" {
		t.Fatalf("after expiry seat status = %q, want available", s)
	}
	e.hold(b, showID, 15)
	if n := e.count(`SELECT count(*) FROM hold_seats hs JOIN holds h ON h.id = hs.hold_id
		WHERE hs.show_id = $1 AND hs.seat_id = 15 AND h.user_id = $2`, showID, b.user.ID); n != 1 {
		t.Fatalf("B's hold rows on seat 15 = %d, want 1", n)
	}
	// A's stale row for seat 15 was replaced; A's other seat stays as an inert expired row.
	if n := e.count(`SELECT count(*) FROM hold_seats WHERE hold_id = $1 AND seat_id = 15`, holdA); n != 0 {
		t.Fatalf("A still has a hold row on seat 15")
	}
	// Cleanup purges stale rows (with its 1-minute grace period).
	_, _ = e.pool.Exec(e.ctx, `UPDATE holds SET expires_at = now() - interval '2 minutes' WHERE id = $1`, holdA)
	if _, err := e.stores.Seats.PurgeExpiredHoldSeats(e.ctx); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT count(*) FROM hold_seats WHERE hold_id = $1`, holdA); n != 0 {
		t.Fatalf("cleanup left %d rows", n)
	}
}

// Required test 4: payment failure creates no booking and keeps the hold.
func TestPaymentFailureCreatesNoBooking(t *testing.T) {
	e := setup(t)
	a := e.customer("Alice")
	showID := e.defaultShow()
	holdID := e.hold(a, showID, 30, 31)

	r := a.post("/api/holds/"+holdID+"/payment", map[string]string{"outcome": "failure"})
	expectStatus(t, r, http.StatusPaymentRequired)

	if n := e.count(`SELECT count(*) FROM bookings`); n != 0 {
		t.Fatalf("bookings = %d, want 0", n)
	}
	if n := e.count(`SELECT count(*) FROM booking_seats`); n != 0 {
		t.Fatalf("booking_seats = %d, want 0", n)
	}
	if n := e.count(`SELECT count(*) FROM holds WHERE id = $1 AND status = 'ACTIVE' AND expires_at > now()`, holdID); n != 1 {
		t.Fatal("hold should still be active after a failed payment")
	}
	if n := e.count(`SELECT count(*) FROM hold_seats WHERE hold_id = $1`, holdID); n != 2 {
		t.Fatalf("hold seats = %d, want 2", n)
	}
	if n := e.count(`SELECT count(*) FROM payment_attempts WHERE hold_id = $1 AND outcome = 'FAILURE'`, holdID); n != 1 {
		t.Fatalf("failure attempts = %d, want 1", n)
	}

	// Try again -> success.
	r = a.post("/api/holds/"+holdID+"/payment", map[string]string{"outcome": "success"})
	expectStatus(t, r, http.StatusCreated)
	if n := e.count(`SELECT count(*) FROM booking_seats WHERE active`); n != 2 {
		t.Fatalf("booking_seats = %d, want 2", n)
	}
	if n := e.count(`SELECT count(*) FROM hold_seats WHERE hold_id = $1`, holdID); n != 0 {
		t.Fatal("hold seats should be released after confirmation")
	}
}

// Required test 5: payment after the hold expired is rejected (success and failure alike).
func TestPaymentAfterExpiryRejected(t *testing.T) {
	e := setup(t)
	a := e.customer("Alice")
	showID := e.defaultShow()
	holdID := e.hold(a, showID, 40)
	e.expireHold(holdID)

	r := a.post("/api/holds/"+holdID+"/payment", map[string]string{"outcome": "success"})
	expectStatus(t, r, http.StatusGone)
	if r.Body["code"] != "HOLD_EXPIRED" {
		t.Fatalf("code = %v, want HOLD_EXPIRED", r.Body["code"])
	}
	expectStatus(t, a.post("/api/holds/"+holdID+"/payment", map[string]string{"outcome": "failure"}), http.StatusGone)
	if n := e.count(`SELECT count(*) FROM bookings`); n != 0 {
		t.Fatalf("bookings = %d, want 0", n)
	}
}

// Required test 6: users can only see and act on their own bookings and holds.
func TestUserCannotAccessAnotherUsersBooking(t *testing.T) {
	e := setup(t)
	a, b := e.customer("Alice"), e.customer("Bob")
	showID := e.defaultShow()
	holdID := e.hold(a, showID, 50)

	// B cannot see or pay for A's hold.
	expectStatus(t, b.get("/api/holds/"+holdID), http.StatusNotFound)
	expectStatus(t, b.post("/api/holds/"+holdID+"/payment", map[string]string{"outcome": "success"}), http.StatusNotFound)
	if n := e.count(`SELECT count(*) FROM bookings`); n != 0 {
		t.Fatal("B's payment attempt on A's hold created a booking")
	}

	r := a.post("/api/holds/"+holdID+"/payment", map[string]string{"outcome": "success"})
	expectStatus(t, r, http.StatusCreated)
	bookingID := int64(r.Body["booking"].(map[string]any)["id"].(float64))

	expectStatus(t, a.get(fmt.Sprintf("/api/bookings/%d", bookingID)), http.StatusOK)
	expectStatus(t, b.get(fmt.Sprintf("/api/bookings/%d", bookingID)), http.StatusNotFound)
	expectStatus(t, b.post(fmt.Sprintf("/api/bookings/%d/cancel", bookingID), nil), http.StatusNotFound)
	expectStatus(t, e.anon().get(fmt.Sprintf("/api/bookings/%d", bookingID)), http.StatusUnauthorized)

	list := b.get("/api/bookings")
	expectStatus(t, list, http.StatusOK)
	if n := len(list.Body["bookings"].([]any)); n != 0 {
		t.Fatalf("B sees %d bookings, want 0", n)
	}
	if n := e.count(`SELECT count(*) FROM bookings WHERE id = $1 AND status = 'CONFIRMED'`, bookingID); n != 1 {
		t.Fatal("A's booking must still be confirmed")
	}
}

// Required test 10: more than six seats is rejected and nothing is held.
func TestMoreThanSixSeatsRejected(t *testing.T) {
	e := setup(t)
	a := e.customer("Alice")
	showID := e.defaultShow()

	r := a.post(fmt.Sprintf("/api/shows/%d/holds", showID), map[string]any{"seatIds": []int{1, 2, 3, 4, 5, 6, 7}})
	expectStatus(t, r, http.StatusBadRequest)
	if r.Body["code"] != "TOO_MANY_SEATS" {
		t.Fatalf("code = %v", r.Body["code"])
	}
	if n := e.count(`SELECT count(*) FROM holds`); n != 0 {
		t.Fatalf("holds = %d, want 0", n)
	}
	// Exactly six is fine.
	e.hold(a, showID, 1, 2, 3, 4, 5, 6)

	// Holding again replaces the previous hold, so the limit cannot be bypassed by repetition.
	e.hold(a, showID, 11, 12, 13, 14, 15, 16)
	if n := e.count(`SELECT count(*) FROM hold_seats hs JOIN holds h ON h.id = hs.hold_id
		WHERE h.user_id = $1 AND h.status = 'ACTIVE'`, a.user.ID); n != 6 {
		t.Fatalf("user holds %d seats, want 6", n)
	}
	// Invalid inputs.
	expectStatus(t, a.post(fmt.Sprintf("/api/shows/%d/holds", showID), map[string]any{"seatIds": []int{}}), 400)
	expectStatus(t, a.post(fmt.Sprintf("/api/shows/%d/holds", showID), map[string]any{"seatIds": []int{81}}), 400)
	expectStatus(t, a.post(fmt.Sprintf("/api/shows/%d/holds", showID), map[string]any{"seatIds": []int{3, 3}}), 400)
}

func TestHoldRejectedForStartedShowAndUnauthenticated(t *testing.T) {
	e := setup(t)
	a := e.customer("Alice")
	movieID := e.movie("Old", 60)
	var past int64
	_ = e.pool.QueryRow(e.ctx, `INSERT INTO shows (movie_id, start_time, end_time, price_paise)
		VALUES ($1, now() - interval '10 minutes', now() + interval '50 minutes', 100) RETURNING id`, movieID).Scan(&past)
	expectStatus(t, a.post(fmt.Sprintf("/api/shows/%d/holds", past), map[string]any{"seatIds": []int{1}}), http.StatusGone)

	upcoming := e.show(movieID, 3*time.Hour)
	expectStatus(t, e.anon().post(fmt.Sprintf("/api/shows/%d/holds", upcoming), map[string]any{"seatIds": []int{1}}), http.StatusUnauthorized)

	// Started shows are not listed on the movie page.
	r := a.get(fmt.Sprintf("/api/movies/%d", movieID))
	list := r.Body["shows"].([]any)
	if len(list) != 1 || int64(list[0].(map[string]any)["id"].(float64)) != upcoming {
		t.Fatalf("movie page lists %v, want only show %d", list, upcoming)
	}
}

func TestRemainingSeatsAndBookingsOrder(t *testing.T) {
	e := setup(t)
	a, b := e.customer("Alice"), e.customer("Bob")
	movieID := e.movie("Count Movie", 60)
	later := e.show(movieID, 48*time.Hour)
	sooner := e.show(movieID, 24*time.Hour)

	e.hold(b, sooner, 1, 2) // held seats count as unavailable
	h := e.hold(a, sooner, 3)
	expectStatus(t, a.post("/api/holds/"+h+"/payment", map[string]string{"outcome": "success"}), 201)
	h = e.hold(a, later, 3)
	expectStatus(t, a.post("/api/holds/"+h+"/payment", map[string]string{"outcome": "success"}), 201)

	r := e.anon().get(fmt.Sprintf("/api/movies/%d", movieID))
	first := r.Body["shows"].([]any)[0].(map[string]any)
	if int64(first["id"].(float64)) != sooner || first["remainingSeats"].(float64) != 77 {
		t.Fatalf("first show = %v, want id %d with 77 remaining", first, sooner)
	}

	// Make the "later" show a past one, then check ordering: upcoming first, past last.
	_, err := e.pool.Exec(e.ctx, `UPDATE shows SET start_time = now() - interval '1 day', end_time = now() - interval '23 hours' WHERE id = $1`, later)
	if err != nil {
		t.Fatal(err)
	}
	list := a.get("/api/bookings").Body["bookings"].([]any)
	if len(list) != 2 || int64(list[0].(map[string]any)["showId"].(float64)) != sooner {
		t.Fatalf("bookings order wrong: %v", list)
	}
}

// Optional feature: cancellation releases seats and is blocked within 2 hours of the show.
func TestCancellation(t *testing.T) {
	e := setup(t)
	a, b := e.customer("Alice"), e.customer("Bob")
	movieID := e.movie("Cancel Movie", 60)
	showID := e.show(movieID, 24*time.Hour)

	h := e.hold(a, showID, 20)
	r := a.post("/api/holds/"+h+"/payment", map[string]string{"outcome": "success"})
	bookingID := int64(r.Body["booking"].(map[string]any)["id"].(float64))

	expectStatus(t, a.post(fmt.Sprintf("/api/bookings/%d/cancel", bookingID), nil), 200)
	expectStatus(t, a.post(fmt.Sprintf("/api/bookings/%d/cancel", bookingID), nil), 409)
	if s := e.seatStatus(b, showID, 20); s != "available" {
		t.Fatalf("seat after cancel = %q", s)
	}
	e.hold(b, showID, 20) // seat can be re-booked
	if n := e.count(`SELECT count(*) FROM booking_seats WHERE booking_id = $1 AND active`, bookingID); n != 0 {
		t.Fatal("cancelled booking still has active seats")
	}

	soon := e.show(movieID, 90*time.Minute)
	h = e.hold(a, soon, 1)
	r = a.post("/api/holds/"+h+"/payment", map[string]string{"outcome": "success"})
	soonBooking := int64(r.Body["booking"].(map[string]any)["id"].(float64))
	r = a.post(fmt.Sprintf("/api/bookings/%d/cancel", soonBooking), nil)
	expectStatus(t, r, 409)
	if r.Body["code"] != "CANCELLATION_CLOSED" {
		t.Fatalf("code = %v", r.Body["code"])
	}
}
