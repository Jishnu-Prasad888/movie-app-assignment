package tests

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"yozigo/seeds"
)

// Required test 7: customers get 403 from every admin endpoint, and nothing changes.
func TestCustomerCannotCallAdminEndpoints(t *testing.T) {
	e := setup(t)
	c := e.customer("Carol")
	movieID := e.movie("Admin Only", 100)
	showID := e.show(movieID, 24*time.Hour)

	tomorrow := time.Now().Add(72 * time.Hour).Format("2006-01-02")
	cases := []struct {
		method, path string
		body         any
	}{
		{"GET", "/api/admin/shows", nil},
		{"GET", fmt.Sprintf("/api/admin/shows/%d", showID), nil},
		{"POST", "/api/admin/shows", map[string]any{"movieId": movieID, "date": tomorrow, "startTime": "10:00", "pricePaise": 10000}},
		{"DELETE", fmt.Sprintf("/api/admin/shows/%d", showID), nil},
		{"POST", "/api/admin/movies", map[string]any{"title": "X", "language": "Hindi", "durationMinutes": 90, "rating": "U"}},
		{"PUT", fmt.Sprintf("/api/admin/movies/%d", movieID), map[string]any{"title": "Hacked", "language": "Hindi", "durationMinutes": 90, "rating": "U"}},
		{"DELETE", fmt.Sprintf("/api/admin/movies/%d", movieID), nil},
	}
	anon := e.anon()
	for _, tc := range cases {
		expectStatus(t, c.do(tc.method, tc.path, tc.body), http.StatusForbidden)
		expectStatus(t, anon.do(tc.method, tc.path, tc.body), http.StatusUnauthorized)
	}
	if n := e.count(`SELECT count(*) FROM movies WHERE title = 'Admin Only'`); n != 1 {
		t.Fatal("movie was modified or deleted by a customer")
	}
	if n := e.count(`SELECT count(*) FROM shows`); n != 1 {
		t.Fatal("shows changed by a customer")
	}

	// An admin can use them.
	admin := e.admin()
	expectStatus(t, admin.get("/api/admin/shows"), http.StatusOK)
}

// Required test 8: overlapping shows are rejected (single screen), with timezone-correct input.
func TestOverlappingShowsRejected(t *testing.T) {
	e := setup(t)
	admin := e.admin()
	movieID := e.movie("Long Film", 150) // 2h30
	day := time.Now().In(e.cfg.CinemaTimezone).AddDate(0, 0, 3).Format("2006-01-02")

	create := func(start string) response {
		return admin.post("/api/admin/shows", map[string]any{"movieId": movieID, "date": day, "startTime": start, "pricePaise": 25000})
	}
	expectStatus(t, create("14:00"), http.StatusCreated) // 14:00-16:30 IST

	for _, start := range []string{"14:00", "13:00", "15:00", "16:29", "11:31"} {
		r := create(start)
		expectStatus(t, r, http.StatusConflict)
		if r.Body["code"] != "SHOW_OVERLAP" {
			t.Fatalf("%s: code = %v", start, r.Body["code"])
		}
	}
	// Back-to-back is allowed: [11:30, 14:00) and [16:30, 19:00).
	expectStatus(t, create("16:30"), http.StatusCreated)
	expectStatus(t, create("11:30"), http.StatusCreated)
	if n := e.count(`SELECT count(*) FROM shows`); n != 3 {
		t.Fatalf("shows = %d, want 3", n)
	}

	// The stored instant is 14:00 in Asia/Kolkata = 08:30 UTC.
	if n := e.count(`SELECT count(*) FROM shows WHERE to_char(start_time AT TIME ZONE 'UTC', 'HH24:MI') = '08:30'`); n != 1 {
		t.Fatal("14:00 IST was not stored as 08:30 UTC")
	}

	// The database constraint rejects overlaps even when the application check is bypassed.
	_, err := e.pool.Exec(e.ctx, `INSERT INTO shows (movie_id, start_time, end_time, price_paise)
		SELECT movie_id, start_time + interval '1 minute', end_time, 100 FROM shows ORDER BY id LIMIT 1`)
	if err == nil {
		t.Fatal("raw overlapping insert succeeded; exclusion constraint missing")
	}

	// Lengthening the movie so that 14:00 would run past 16:30 is rejected too.
	r := admin.put(fmt.Sprintf("/api/admin/movies/%d", movieID), map[string]any{
		"title": "Long Film", "language": "English", "durationMinutes": 170, "rating": "UA"})
	expectStatus(t, r, http.StatusConflict)
}

func TestAdminShowLifecycleAndStats(t *testing.T) {
	e := setup(t)
	admin := e.admin()
	c := e.customer("Dev")
	movieID := e.movie("Stats Movie", 100)
	showID := e.show(movieID, 24*time.Hour)
	emptyShow := e.show(movieID, 48*time.Hour)

	h := e.hold(c, showID, 1, 2, 3, 4)
	expectStatus(t, c.post("/api/holds/"+h+"/payment", map[string]string{"outcome": "success"}), 201)

	r := admin.get(fmt.Sprintf("/api/admin/shows/%d", showID))
	expectStatus(t, r, 200)
	show := r.Body["show"].(map[string]any)
	if show["bookedSeats"].(float64) != 4 || show["occupancyPercent"].(float64) != 5 || show["revenuePaise"].(float64) != 80000 {
		t.Fatalf("stats wrong: %v", show)
	}
	bk := r.Body["bookings"].([]any)
	if len(bk) != 1 || bk[0].(map[string]any)["customerName"] != "Dev" {
		t.Fatalf("bookings = %v", bk)
	}

	r = admin.del(fmt.Sprintf("/api/admin/shows/%d", showID))
	expectStatus(t, r, http.StatusConflict)
	expectStatus(t, admin.del(fmt.Sprintf("/api/admin/movies/%d", movieID)), http.StatusConflict)
	expectStatus(t, admin.del(fmt.Sprintf("/api/admin/shows/%d", emptyShow)), http.StatusNoContent)
	if n := e.count(`SELECT count(*) FROM shows`); n != 1 {
		t.Fatalf("shows = %d, want 1", n)
	}

	// Movie validation.
	r = admin.post("/api/admin/movies", map[string]any{"title": "", "language": "", "durationMinutes": 0, "rating": "PG"})
	expectStatus(t, r, 400)
	if len(r.Body["fields"].(map[string]any)) != 4 {
		t.Fatalf("fields = %v", r.Body["fields"])
	}
	r = admin.post("/api/admin/movies", map[string]any{"title": "New", "language": "Kannada", "durationMinutes": 120, "rating": "ua"})
	expectStatus(t, r, 201)
	newID := int64(r.Body["movie"].(map[string]any)["id"].(float64))
	expectStatus(t, admin.del(fmt.Sprintf("/api/admin/movies/%d", newID)), http.StatusNoContent)
}

// Required test 9: duplicate email signup is rejected (case-insensitively).
func TestDuplicateEmailSignupRejected(t *testing.T) {
	e := setup(t)
	body := map[string]string{"name": "Dana", "email": "dana@example.com", "password": "password123"}
	expectStatus(t, e.anon().post("/api/auth/signup", body), http.StatusCreated)

	body["email"] = "  DANA@Example.com "
	r := e.anon().post("/api/auth/signup", body)
	expectStatus(t, r, http.StatusConflict)
	if r.Body["code"] != "EMAIL_TAKEN" {
		t.Fatalf("code = %v", r.Body["code"])
	}
	if n := e.count(`SELECT count(*) FROM users WHERE email = 'dana@example.com'`); n != 1 {
		t.Fatalf("users = %d, want 1", n)
	}
}

func TestSignupValidationAndNeverAdmin(t *testing.T) {
	e := setup(t)
	a := e.anon()
	expectStatus(t, a.post("/api/auth/signup", map[string]string{"name": "Eve", "email": "eve@example.com", "password": "short"}), 400)
	expectStatus(t, a.post("/api/auth/signup", map[string]string{"name": "", "email": "eve@example.com", "password": "password123"}), 400)
	expectStatus(t, a.post("/api/auth/signup", map[string]string{"name": "Eve", "email": "not-an-email", "password": "password123"}), 400)
	expectStatus(t, a.post("/api/auth/signup", map[string]string{"name": "Eve", "email": "admin@yozigo.test", "password": "password123"}), 400)
	// A smuggled role field is rejected outright.
	expectStatus(t, a.post("/api/auth/signup", map[string]any{"name": "Eve", "email": "eve@example.com", "password": "password123", "role": "admin"}), 400)
	if n := e.count(`SELECT count(*) FROM users WHERE role = 'admin'`); n != 0 {
		t.Fatal("signup created an admin")
	}
	r := a.post("/api/auth/signup", map[string]string{"name": "Eve", "email": "eve@example.com", "password": "password123"})
	expectStatus(t, r, 201)
	if r.Body["user"].(map[string]any)["role"] != "customer" {
		t.Fatal("new user is not a customer")
	}
}

func TestLoginErrorsAreGenericAndLogoutWorks(t *testing.T) {
	e := setup(t)
	c := e.customer("Finn")
	wrongPw := e.anon().post("/api/auth/login", map[string]string{"email": "finn@example.com", "password": "wrongpass1"})
	noUser := e.anon().post("/api/auth/login", map[string]string{"email": "nobody@example.com", "password": "wrongpass1"})
	expectStatus(t, wrongPw, 401)
	expectStatus(t, noUser, 401)
	if wrongPw.Raw != noUser.Raw {
		t.Fatalf("login errors differ: %q vs %q", wrongPw.Raw, noUser.Raw)
	}

	expectStatus(t, c.get("/api/bookings"), 200)
	expectStatus(t, c.post("/api/auth/logout", nil), 204)
	expectStatus(t, c.get("/api/bookings"), 401)
	if n := e.count(`SELECT count(*) FROM sessions`); n != 0 {
		t.Fatalf("sessions after logout = %d", n)
	}
}

func TestCSRFHeaderRequired(t *testing.T) {
	e := setup(t)
	res, err := http.Post(e.srv.URL+"/api/auth/demo-login", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", res.StatusCode)
	}
}

func TestDemoLogin(t *testing.T) {
	e := setup(t)
	c := e.anon()

	// Before seeding: a clear error, and nothing is created.
	r := c.post("/api/auth/demo-login", map[string]string{"account": "admin"})
	expectStatus(t, r, http.StatusNotFound)
	if r.Body["code"] != "DEMO_ACCOUNT_MISSING" {
		t.Fatalf("code = %v", r.Body["code"])
	}
	if n := e.count(`SELECT count(*) FROM users`); n != 0 {
		t.Fatal("demo login created a user")
	}

	if err := seeds.Run(e.ctx, e.pool, e.cfg.DemoPassword, e.cfg.CinemaTimezone); err != nil {
		t.Fatal(err)
	}
	// Seeding twice is harmless (idempotent).
	if err := seeds.Run(e.ctx, e.pool, e.cfg.DemoPassword, e.cfg.CinemaTimezone); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT count(*) FROM users WHERE email LIKE '%@yozigo.test'`); n != 4 {
		t.Fatalf("demo users = %d, want 4", n)
	}

	for key, role := range map[string]string{"admin": "admin", "customer1": "customer", "customer2": "customer", "customer3": "customer"} {
		c := e.anon()
		r := c.post("/api/auth/demo-login", map[string]string{"account": key})
		expectStatus(t, r, 200)
		if got := r.Body["user"].(map[string]any)["role"]; got != role {
			t.Fatalf("%s: role %v, want %s", key, got, role)
		}
		me := c.get("/api/auth/me")
		if me.Body["user"].(map[string]any)["role"] != role {
			t.Fatalf("%s: /me role mismatch", key)
		}
		// Normal authorization still applies after demo login.
		want := http.StatusForbidden
		if role == "admin" {
			want = http.StatusOK
		}
		expectStatus(t, c.get("/api/admin/shows"), want)
	}

	// Only allowlisted keys; arbitrary identities are rejected.
	for _, body := range []any{
		map[string]string{"account": "root"},
		map[string]string{"account": "admin@yozigo.test"},
		map[string]any{"account": "customer1", "role": "admin"},
		map[string]any{"userId": 1},
	} {
		expectStatus(t, e.anon().post("/api/auth/demo-login", body), http.StatusBadRequest)
	}

	// A demo slot whose account has the wrong role is refused (never escalates privileges).
	_, _ = e.pool.Exec(e.ctx, `UPDATE users SET role = 'admin' WHERE email = 'customer3@yozigo.test'`)
	expectStatus(t, e.anon().post("/api/auth/demo-login", map[string]string{"account": "customer3"}), http.StatusConflict)

	// Disabled -> 404, even for valid keys.
	e.cfg.DemoLoginEnabled = false
	srv := e.srv
	srv.Config.Handler = newHandlerForTest(e)
	expectStatus(t, e.anon().post("/api/auth/demo-login", map[string]string{"account": "customer1"}), http.StatusNotFound)

	// The seeded demo accounts also work with the normal login form.
	expectStatus(t, e.anon().post("/api/auth/login", map[string]string{"email": "customer1@yozigo.test", "password": "yozigo-demo"}), 200)
}
