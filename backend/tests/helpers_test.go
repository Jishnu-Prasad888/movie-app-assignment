// Package tests contains integration tests that run against a real PostgreSQL database.
//
// Set TEST_DATABASE_URL to a database that may be wiped, e.g.
//
//	TEST_DATABASE_URL=postgres://yozigo:yozigo_dev_password@localhost:5433/yozigo_test?sslmode=disable go test ./...
//
// The schema is dropped and re-migrated once per run, and every test starts from empty tables.
// Tests are not run in parallel because they share the database.
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"yozigo/internal/auth"
	"yozigo/internal/config"
	"yozigo/internal/database"
	"yozigo/internal/server"
	"yozigo/migrations"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	// Keep test output readable: request logs are noise here.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		fmt.Println("TEST_DATABASE_URL not set: integration tests will be SKIPPED")
		os.Exit(m.Run())
	}
	ctx := context.Background()
	pool, err := database.Connect(ctx, url)
	if err != nil {
		fmt.Println("connect:", err)
		os.Exit(1)
	}
	// Fresh schema for every run, so migrations themselves are exercised too.
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		fmt.Println("reset schema:", err)
		os.Exit(1)
	}
	if err := database.Migrate(ctx, pool, migrations.FS); err != nil {
		fmt.Println("migrate:", err)
		os.Exit(1)
	}
	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// env is one test's isolated application instance.
type env struct {
	t      *testing.T
	ctx    context.Context
	pool   *pgxpool.Pool
	cfg    config.Config
	stores *server.Stores
	srv    *httptest.Server
}

func setup(t *testing.T) *env {
	t.Helper()
	if testPool == nil {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	_, err := testPool.Exec(ctx, `TRUNCATE users, sessions, movies, shows, show_seats, holds, hold_seats,
		bookings, booking_seats, payment_attempts RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
	loc, _ := time.LoadLocation("Asia/Kolkata")
	cfg := config.Config{
		AppEnv:           "test",
		DemoLoginEnabled: true,
		DemoPassword:     "yozigo-demo",
		SessionTTL:       time.Hour,
		HoldDuration:     5 * time.Minute,
		CinemaTimezone:   loc,
	}
	stores := server.NewStores(testPool, cfg)
	srv := httptest.NewServer(server.New(cfg, testPool, stores))
	t.Cleanup(srv.Close)
	return &env{t: t, ctx: ctx, pool: testPool, cfg: cfg, stores: stores, srv: srv}
}

// ---- HTTP client ---------------------------------------------------------------------------

// client is a browser-like HTTP client with its own cookie jar (= its own session).
type client struct {
	e    *env
	http *http.Client
	user *auth.User
}

func (e *env) anon() *client {
	jar, _ := cookiejar.New(nil)
	return &client{e: e, http: &http.Client{Jar: jar, Timeout: 30 * time.Second}}
}

type response struct {
	Status int
	Body   map[string]any
	Raw    string
}

func (c *client) do(method, path string, body any) response {
	c.e.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.e.srv.URL+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	res, err := c.http.Do(req)
	if err != nil {
		c.e.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := response{Status: res.StatusCode, Raw: string(raw)}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out.Body)
	}
	return out
}

func (c *client) get(path string) response            { return c.do(http.MethodGet, path, nil) }
func (c *client) post(path string, body any) response { return c.do(http.MethodPost, path, body) }
func (c *client) del(path string) response            { return c.do(http.MethodDelete, path, nil) }
func (c *client) put(path string, body any) response  { return c.do(http.MethodPut, path, body) }

func expectStatus(t *testing.T, r response, want int) {
	t.Helper()
	if r.Status != want {
		t.Fatalf("expected HTTP %d, got %d: %s", want, r.Status, r.Raw)
	}
}

// ---- Fixtures ------------------------------------------------------------------------------

// customer signs up a new customer through the API and returns a logged-in client.
func (e *env) customer(name string) *client {
	e.t.Helper()
	c := e.anon()
	email := strings.ToLower(strings.ReplaceAll(name, " ", ".")) + "@example.com"
	r := c.post("/api/auth/signup", map[string]string{"name": name, "email": email, "password": "password123"})
	expectStatus(e.t, r, http.StatusCreated)
	u, err := e.stores.Auth.UserByEmail(e.ctx, email)
	if err != nil || u == nil {
		e.t.Fatalf("load user: %v", err)
	}
	c.user = u
	return c
}

// admin inserts an admin directly (signup can never create one) and logs in normally.
func (e *env) admin() *client {
	e.t.Helper()
	hash, _ := auth.HashPassword("adminpass1")
	if _, err := e.pool.Exec(e.ctx, `INSERT INTO users (name, email, password_hash, role)
		VALUES ('Test Admin', 'admin@example.com', $1, 'admin')`, hash); err != nil {
		e.t.Fatal(err)
	}
	c := e.anon()
	expectStatus(e.t, c.post("/api/auth/login", map[string]string{"email": "admin@example.com", "password": "adminpass1"}), 200)
	c.user, _ = e.stores.Auth.UserByEmail(e.ctx, "admin@example.com")
	return c
}

func (e *env) movie(title string, durationMin int) int64 {
	e.t.Helper()
	var id int64
	if err := e.pool.QueryRow(e.ctx, `INSERT INTO movies (title, language, duration_minutes, rating)
		VALUES ($1, 'English', $2, 'UA') RETURNING id`, title, durationMin).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// show schedules a show starting `in` from now, priced at ₹200.
func (e *env) show(movieID int64, in time.Duration) int64 {
	e.t.Helper()
	s, err := e.stores.Shows.Create(e.ctx, movieID, time.Now().Add(in).Truncate(time.Minute), 20000)
	if err != nil {
		e.t.Fatalf("create show: %v", err)
	}
	return s.ID
}

// defaultShow creates a movie and a show tomorrow.
func (e *env) defaultShow() int64 {
	return e.show(e.movie("Test Movie", 120), 24*time.Hour)
}

func (e *env) hold(c *client, showID int64, seatIDs ...int) string {
	e.t.Helper()
	r := c.post(fmt.Sprintf("/api/shows/%d/holds", showID), map[string]any{"seatIds": seatIDs})
	expectStatus(e.t, r, http.StatusCreated)
	return r.Body["hold"].(map[string]any)["id"].(string)
}

// expireHold moves a hold's expiry into the past, simulating 5 minutes passing.
func (e *env) expireHold(holdID string) {
	e.t.Helper()
	if _, err := e.pool.Exec(e.ctx, `UPDATE holds SET expires_at = now() - interval '1 second' WHERE id = $1`, holdID); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.pool.QueryRow(e.ctx, query, args...).Scan(&n); err != nil {
		e.t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func (e *env) seatStatus(c *client, showID int64, seatID int) string {
	e.t.Helper()
	r := c.get(fmt.Sprintf("/api/shows/%d/seats", showID))
	expectStatus(e.t, r, 200)
	for _, s := range r.Body["seats"].([]any) {
		m := s.(map[string]any)
		if int(m["id"].(float64)) == seatID {
			return m["status"].(string)
		}
	}
	e.t.Fatalf("seat %d not in map", seatID)
	return ""
}

// newHandlerForTest rebuilds the router from e.cfg (used after changing config mid-test).
func newHandlerForTest(e *env) http.Handler {
	return server.New(e.cfg, e.pool, e.stores)
}
