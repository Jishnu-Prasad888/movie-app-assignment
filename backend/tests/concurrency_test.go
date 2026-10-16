package tests

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"yozigo/internal/apperr"
	"yozigo/internal/database"
	"yozigo/internal/seats"
)

// Required test 1: users racing for the same seat — exactly one wins.
//
// Each contender calls AcquireHold from its own goroutine; each call runs in its own database
// transaction on its own pooled connection. A barrier (close(start)) releases all goroutines at
// once so they really do race. Repeated over many rounds to make a lucky pass implausible.
func TestConcurrentHoldSameSeat_ExactlyOneSucceeds(t *testing.T) {
	e := setup(t)
	const contenders, rounds = 8, 20

	users := make([]*client, contenders)
	for i := range users {
		users[i] = e.customer(fmt.Sprintf("Racer %d", i))
	}
	movieID := e.movie("Race Movie", 90)

	for round := 0; round < rounds; round++ {
		showID := e.show(movieID, time.Duration(24+round*2)*time.Hour)
		seat := 1 + round%80

		start := make(chan struct{})
		var wg sync.WaitGroup
		var wins, conflicts atomic.Int32
		winner := make(chan int64, contenders)
		for _, u := range users {
			wg.Add(1)
			go func(userID int64) {
				defer wg.Done()
				<-start
				_, err := e.stores.Seats.AcquireHold(context.Background(), userID, showID, []int{seat})
				switch {
				case err == nil:
					wins.Add(1)
					winner <- userID
				case apperr.IsKind(err, apperr.Conflict):
					conflicts.Add(1)
				default:
					t.Errorf("unexpected error: %v", err)
				}
			}(u.user.ID)
		}
		close(start)
		wg.Wait()
		close(winner)

		if wins.Load() != 1 || conflicts.Load() != contenders-1 {
			t.Fatalf("round %d: wins=%d conflicts=%d, want 1 and %d", round, wins.Load(), conflicts.Load(), contenders-1)
		}
		// Assert the actual database state, not just return values.
		winnerID := <-winner
		var holders []int64
		rows, _ := e.pool.Query(e.ctx, `SELECT h.user_id FROM hold_seats hs JOIN holds h ON h.id = hs.hold_id
			WHERE hs.show_id = $1 AND hs.seat_id = $2 AND h.status = 'ACTIVE'`, showID, seat)
		holders, err := pgx.CollectRows(rows, pgx.RowTo[int64])
		if err != nil {
			t.Fatal(err)
		}
		if len(holders) != 1 || holders[0] != winnerID {
			t.Fatalf("round %d: hold rows for seat = %v, want exactly [%d]", round, holders, winnerID)
		}
	}
}

// Overlapping (not identical) seat sets also race correctly and never deadlock, because
// every transaction locks seats in ascending seat_id order.
func TestConcurrentHoldOverlappingSets_NoDeadlockAllOrNothing(t *testing.T) {
	e := setup(t)
	a, b := e.customer("Alpha"), e.customer("Beta")
	showID := e.defaultShow()

	for round := 0; round < 15; round++ {
		_, _ = e.pool.Exec(e.ctx, `DELETE FROM hold_seats; DELETE FROM holds;`)
		start := make(chan struct{})
		var wg sync.WaitGroup
		errs := make([]error, 2)
		sets := [][]int{{10, 11, 12}, {12, 11, 13}} // deliberately unsorted input
		for i, u := range []*client{a, b} {
			wg.Add(1)
			go func(i int, userID int64) {
				defer wg.Done()
				<-start
				_, errs[i] = e.stores.Seats.AcquireHold(context.Background(), userID, showID, sets[i])
			}(i, u.user.ID)
		}
		close(start)
		wg.Wait()
		ok := 0
		for _, err := range errs {
			if err == nil {
				ok++
			} else if !apperr.IsKind(err, apperr.Conflict) {
				t.Fatalf("round %d: unexpected error (deadlock?): %v", round, err)
			}
		}
		if ok != 1 {
			t.Fatalf("round %d: %d holds succeeded, want 1", round, ok)
		}
		// All-or-nothing: the loser holds nothing, the winner holds exactly 3 seats.
		if n := e.count(`SELECT count(*) FROM hold_seats WHERE show_id = $1`, showID); n != 3 {
			t.Fatalf("round %d: %d held seats, want 3", round, n)
		}
	}
}

// Deterministic demonstration of the row lock: while transaction A holds the lock on a seat,
// B's AcquireHold blocks; once A commits its hold, B wakes up, re-checks and gets a conflict.
func TestHoldWaitsForRowLock(t *testing.T) {
	e := setup(t)
	a, b := e.customer("Alice"), e.customer("Bob")
	showID := e.defaultShow()

	txA, err := e.pool.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer txA.Rollback(e.ctx)
	if _, err := seats.LockShowSeats(e.ctx, txA, showID, []int{5}); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := e.stores.Seats.AcquireHold(context.Background(), b.user.ID, showID, []int{5})
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("B finished while A held the lock (err=%v); expected it to block", err)
	case <-time.After(300 * time.Millisecond):
	}

	// A completes a hold of seat 5 in its transaction and commits.
	var holdID string
	if err := txA.QueryRow(e.ctx, `INSERT INTO holds (user_id, show_id, expires_at)
		VALUES ($1, $2, now() + interval '5 minutes') RETURNING id::text`, a.user.ID, showID).Scan(&holdID); err != nil {
		t.Fatal(err)
	}
	if _, err := txA.Exec(e.ctx, `INSERT INTO hold_seats (hold_id, show_id, seat_id) VALUES ($1, $2, 5)`, holdID, showID); err != nil {
		t.Fatal(err)
	}
	if err := txA.Commit(e.ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if !apperr.IsKind(err, apperr.Conflict) {
			t.Fatalf("B: want conflict after A committed, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("B still blocked after A committed")
	}
}

// Required test 2a: even if application-level locking were bypassed entirely, the partial
// unique index on booking_seats makes a duplicate active booking impossible. Two raw
// transactions (separate goroutines and connections) insert the same seat concurrently.
func TestUniqueConstraintPreventsDuplicateBooking_EvenWithoutLocking(t *testing.T) {
	e := setup(t)
	a, b := e.customer("Alice"), e.customer("Bob")
	showID := e.defaultShow()

	var bookingIDs [2]int64
	for i, u := range []*client{a, b} {
		if err := e.pool.QueryRow(e.ctx, `INSERT INTO bookings (user_id, show_id, status, total_paise)
			VALUES ($1, $2, 'CONFIRMED', 20000) RETURNING id`, u.user.ID, showID).Scan(&bookingIDs[i]); err != nil {
			t.Fatal(err)
		}
	}

	var ready sync.WaitGroup
	ready.Add(2)
	start := make(chan struct{})
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tx, err := e.pool.Begin(context.Background())
			if err != nil {
				errs[i] = err
				ready.Done()
				return
			}
			defer tx.Rollback(context.Background())
			ready.Done()
			<-start
			if _, err := tx.Exec(context.Background(), `INSERT INTO booking_seats (booking_id, show_id, seat_id)
				VALUES ($1, $2, 42)`, bookingIDs[i], showID); err != nil {
				errs[i] = err
				return
			}
			errs[i] = tx.Commit(context.Background())
		}(i)
	}
	ready.Wait() // both transactions are open
	close(start)
	wg.Wait()

	succeeded, uniqueViolations := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case database.IsPgError(err, database.UniqueViolation):
			uniqueViolations++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 || uniqueViolations != 1 {
		t.Fatalf("succeeded=%d uniqueViolations=%d, want 1 and 1", succeeded, uniqueViolations)
	}
	if n := e.count(`SELECT count(*) FROM booking_seats WHERE show_id = $1 AND seat_id = 42 AND active`, showID); n != 1 {
		t.Fatalf("active booking rows for seat = %d, want 1", n)
	}
}

// Required test 2b: two users try to book the same seat through the real flow. A's hold has
// expired and B legitimately holds the seat now; both submit payment at the same moment.
// Only B's booking may exist.
func TestTwoUsersConfirmSameSeat_OnlyOneBooking(t *testing.T) {
	e := setup(t)
	a, b := e.customer("Alice"), e.customer("Bob")
	showID := e.defaultShow()

	holdA := e.hold(a, showID, 7)
	e.expireHold(holdA)
	holdB := e.hold(b, showID, 7)

	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]response, 2)
	for i, pair := range []struct {
		c    *client
		hold string
	}{{a, holdA}, {b, holdB}} {
		wg.Add(1)
		go func(i int, c *client, hold string) {
			defer wg.Done()
			<-start
			results[i] = c.post("/api/holds/"+hold+"/payment", map[string]string{"outcome": "success"})
		}(i, pair.c, pair.hold)
	}
	close(start)
	wg.Wait()

	expectStatus(t, results[0], 410) // A: hold expired
	expectStatus(t, results[1], 201) // B: booked
	if n := e.count(`SELECT count(*) FROM booking_seats WHERE show_id = $1 AND seat_id = 7 AND active`, showID); n != 1 {
		t.Fatalf("active booking rows = %d, want 1", n)
	}
	if n := e.count(`SELECT count(*) FROM bookings WHERE user_id = $1`, a.user.ID); n != 0 {
		t.Fatalf("A has %d bookings, want 0", n)
	}
}

// Duplicate submissions of the same payment (double click, two tabs, retries) create exactly
// one booking and all return the same booking id.
func TestDuplicateConfirmIsIdempotent(t *testing.T) {
	e := setup(t)
	a := e.customer("Alice")
	showID := e.defaultShow()
	holdID := e.hold(a, showID, 1, 2)

	const n = 6
	start := make(chan struct{})
	var wg sync.WaitGroup
	ids := make([]int64, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			res, err := e.stores.Bookings.Confirm(context.Background(), a.user.ID, holdID)
			errs[i] = err
			if err == nil {
				ids[i] = res.BookingID
			}
		}(i)
	}
	close(start)
	wg.Wait()
	for i := range ids {
		if errs[i] != nil {
			t.Fatalf("confirm %d: %v", i, errs[i])
		}
		if ids[i] != ids[0] {
			t.Fatalf("different booking ids returned: %v", ids)
		}
	}
	if c := e.count(`SELECT count(*) FROM bookings WHERE hold_id = $1`, holdID); c != 1 {
		t.Fatalf("bookings for hold = %d, want 1", c)
	}
	if c := e.count(`SELECT count(*) FROM booking_seats WHERE show_id = $1`, showID); c != 2 {
		t.Fatalf("booking_seats = %d, want 2", c)
	}
}
