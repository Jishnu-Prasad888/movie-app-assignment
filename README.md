# Yozigo — Movie Ticket Booking

A full-stack booking app for a small single-screen cinema (80 seats, rows A–H × seats 1–10).

| Layer     | Tech |
|-----------|------|
| Frontend  | React 19 + TypeScript + Vite, Tailwind CSS v4, React Router |
| Backend   | Go (standard library `net/http` router), `pgx/v5` with hand-written SQL |
| Database  | PostgreSQL 17, plain SQL migrations embedded in the binary |
| Auth      | Server-side sessions in an HttpOnly cookie, bcrypt password hashes |
| Local run | Docker Compose (db + api + nginx-served frontend) |
| Tests     | Go unit tests + PostgreSQL integration tests (including concurrency tests) |

---

## 1. Quick start

Prerequisites: **Docker** with the Compose plugin. Go 1.27+ and Node 22+ are needed only for local development without Docker.

```bash
cp .env.example .env          # local settings (demo login enabled, seed on start)
docker compose up --build
```

Then open **http://localhost:3000**.

On start-up the API applies the migrations, then runs the idempotent seed: 4 demo accounts, 6 movies, and four shows a day for the next 7 days. The API is also exposed directly on http://localhost:8080, and PostgreSQL is on `localhost:5433`.

### Reset the database and reseed

```bash
docker compose down -v        # -v deletes the pgdata volume, i.e. ALL data
docker compose up --build     # migrations + seed run again on a clean database
```

To reseed without wiping data, run `docker compose exec api api seed`. The seed is idempotent: demo users are inserted with `ON CONFLICT DO NOTHING`, and movies and shows are inserted only into empty tables.

---

## 2. Demo login

The login page has a **Demo Accounts** panel with four buttons:

| Button                | Key         | Seeded account          | Role     |
|-----------------------|-------------|-------------------------|----------|
| Login as Admin        | `admin`     | `admin@yozigo.test` (Asha Admin) | admin    |
| Login as Customer 1   | `customer1` | `customer1@yozigo.test` (Ravi Kumar) | customer |
| Login as Customer 2   | `customer2` | `customer2@yozigo.test` (Meera Iyer) | customer |
| Login as Customer 3   | `customer3` | `customer3@yozigo.test` (Arjun Singh) | customer |

The same accounts also work with the normal login form, using the `DEMO_PASSWORD` from `.env` (default `yozigo-demo`).

**How it stays safe:**

- **Opt-in:** `POST /api/auth/demo-login` works only when `DEMO_LOGIN_ENABLED=true`. It defaults to `false`, and the API **refuses to start** if it is enabled while `APP_ENV=production`. The panel is hidden when the flag is off (the frontend reads `/api/config`).
- **Allowlist:** the browser sends only a fixed key (`{"account":"customer1"}`). The backend keeps an explicit allowlist (`internal/auth/demo.go`) that maps each key to an email and an expected role. Any other key, user id, email or role field is rejected with 400 (the JSON decoder rejects unknown fields).
- **Nothing is created:** the account must already exist from the seed and have the expected role. A missing account returns a clear `DEMO_ACCOUNT_MISSING` error; a role mismatch is refused.
- **No secrets in the frontend:** no passwords, tokens or signing secrets live there. The result is an ordinary session cookie, so every normal authorization check still applies (customers still get 403 from admin APIs).
- **Reserved domain:** the whole `@yozigo.test` domain is blocked for normal signup, so nobody can pre-register a demo address.

> ⚠️ **Enable demo login only in an isolated demonstration environment.** Anyone who can reach the site can log in as the admin with one click.

---

## 3. Features

**Customers**
- Browse movies: poster or generated placeholder, title, language, duration, rating (U/UA/A) and description.
- Movie page: upcoming shows grouped by date (cinema time zone), each with start time, price and live remaining seats. Shows that have already started are hidden.
- Seat map: 80 seats, a "Screen this way" marker, and a legend for each state (available / selected / your hold / held by another customer / booked). The map polls every 4 s.
- Select 1–6 seats. A 7th is refused with a message, and the selected seat labels and total are shown.
- **Continue** atomically holds all seats for 5 minutes, or none. On conflict, the clashing seats are named, removed from the selection, and the map refreshes.
- Checkout shows the details and a countdown. **Pay** opens a fake gateway with **Payment success** and **Payment failure**:
  - success → booking confirmed → confirmation page (booking ID, movie, date, time, seats, amount, status);
  - failure → no booking, the hold is kept, and **Try again** is offered.
- My Bookings: upcoming first (soonest first), then past (most recent first).
- *Optional, implemented:* cancel a booking until 2 hours before the show; the seats are released transactionally.

**Admins**
- Add, edit and delete movies. Deletion is blocked if any of the movie's shows have bookings.
- Schedule shows (movie, date, start time, price). Overlaps on the single screen are rejected, with the conflicting show named.
- Delete shows; a show that has bookings can't be deleted.
- The dashboard lists all shows with booked/80, occupancy %, and confirmed revenue. It auto-refreshes every 10 s.
- Show detail: stats, a bookings table (booking ID, customer, seats, booking time, status), and a read-only seat map.

---

## 4. Architecture

```
frontend/                     React SPA (served by nginx in Docker; nginx proxies /api → api:8080)
  src/api/                    fetch wrapper + one function per endpoint
  src/hooks/                  useAuth (session state), useApi, useCountdown, useToast
  src/components/             SeatMap, Poster, ProtectedRoute, feedback components
  src/pages/ (+ admin/)       one file per screen
backend/
  cmd/api/main.go             entrypoint: `api` (serve) | `api migrate` | `api seed`; cleanup loop
  internal/config             env configuration
  internal/database           pgx pool, transaction helper, migration runner
  internal/apperr, httpx      typed business errors → HTTP status/JSON mapping
  internal/middleware         logging, panic recovery, CSRF header check, security headers
  internal/auth               signup/login/logout/demo-login, sessions, RequireAuth/RequireAdmin
  internal/movies, shows      catalogue + scheduling (overlap checks)
  internal/seats              seat map, holds, row-locking helper  ← concurrency core
  internal/bookings           payment, confirmation, history, cancellation
  internal/admin              admin endpoints and stats
  internal/server             router: every route declares its protection level
  migrations/                 0001_init.sql (embedded via go:embed)
  seeds/                      deterministic demo data
  tests/                      PostgreSQL integration tests
```

Browser and API share one origin: Vite's dev proxy in development, nginx in Docker. So the session cookie is first-party and no CORS configuration exists.

---

## 5. Database schema

```
users ──< sessions
users ──< holds >── shows >── movies
           │          │
           └─< hold_seats >─┐
                            ├── show_seats (show_id, seat_id)  ── seats (80 rows, A1..H10)
users ──< bookings >── shows │
           └─< booking_seats ┘
holds ──< payment_attempts
```

| Table | Purpose / key constraints |
|-------|---------------------------|
| `users` | `email` unique (stored lower-case → case-insensitive), `role IN ('customer','admin')` |
| `sessions` | `token_hash` = SHA-256 of the cookie token, `expires_at` |
| `movies` | title, description, language, `duration_minutes`, `rating IN ('U','UA','A')`, `poster_url` |
| `seats` | the fixed 80-seat layout; id 1 = A1 … 80 = H10 |
| `shows` | `start_time`, `end_time` (= start + duration), `price_paise`; **`EXCLUDE USING gist (tstzrange(start_time,end_time,'[)') WITH &&)`** prevents overlaps |
| `show_seats` | one row per (show, seat), created by an `AFTER INSERT` trigger on `shows`; these are the rows that get **locked** |
| `holds` | `user_id`, `show_id`, `status ACTIVE/CONFIRMED/RELEASED`, `expires_at` |
| `hold_seats` | **`UNIQUE (show_id, seat_id)`**; composite FKs guarantee the seat belongs to the hold's show |
| `bookings` | `status CONFIRMED/CANCELLED`, `total_paise`, **`hold_id UNIQUE`** (one booking per hold → idempotent payment) |
| `booking_seats` | `active` flag; **partial unique index `(show_id, seat_id) WHERE active`** — the final double-booking safeguard |
| `payment_attempts` | audit log of fake gateway results |

Design notes:
- **Money:** integer paise everywhere, never floats.
- **Time:** all instants are `TIMESTAMPTZ`. Admin input such as "2026-10-20 18:30" is interpreted in `CINEMA_TIMEZONE` (default `Asia/Kolkata`), and the UI formats times in that zone, so overlap checks compare real instants.
- **History is kept:** `bookings.show_id` is `ON DELETE RESTRICT`, so a show with bookings cannot be deleted even by a racing request.

---

## 6. Authentication and authorization

- **Signup:** requires name, email and a password of at least 8 characters (at most 72 bytes, bcrypt's limit). The role is hard-coded to `customer` in SQL, so signup can never create an admin. Duplicate emails get `409 EMAIL_TAKEN`.
- **Passwords:** stored as bcrypt hashes. Login returns the same `401 "Invalid email or password."` for an unknown email or a wrong password. Unknown emails are still checked against a dummy hash, so timing doesn't reveal which emails exist.
- **Sessions:** login creates a random 256-bit token. The browser gets it in a cookie that is `HttpOnly` (unreadable by JavaScript), `SameSite=Lax`, and `Secure` when `COOKIE_SECURE=true`. The database stores only SHA-256(token). Logout deletes the row, so sessions are revocable immediately, unlike a stateless JWT.
- **Identity:** on every request, `auth.Middleware.Authenticate` loads the user and role from the database via the cookie. Handlers take the user id only from that context; no endpoint accepts a user id or role from the client.
- **Middleware:** `RequireAuth` returns 401. `RequireAdmin` returns 401 to anonymous users and **403** to customers. Each route in `internal/server/server.go` declares its wrapper.
- **Ownership:** booking and hold queries include `AND user_id = $currentUser`. Another user's resource returns **404**, so ids can't be probed.
- **CSRF:** SameSite=Lax cookies, plus a required `X-Requested-With: XMLHttpRequest` header on every non-GET request. Cross-site forms can't set it, and cross-origin `fetch` would need a CORS preflight that is never approved.
- **Frontend guards:** `ProtectedRoute` is UX only. Security lives in the API.

---

## 7. How double booking is prevented

**1. One lockable row per seat per show.** `show_seats` has a row for every (show, seat). Every transaction that changes seat state locks the relevant rows first:

```sql
SELECT seat_id FROM show_seats WHERE show_id = $1 AND seat_id = ANY($2)
ORDER BY seat_id FOR UPDATE;
```

A second transaction touching any of those seats blocks until the first commits or rolls back. Under READ COMMITTED it then re-reads and sees the new state, so "check the seat is free, then hold it" can never interleave.

**2. Consistent lock order means no deadlocks.** Rows are always locked in ascending `seat_id`, and resources always in the same order:
`users row (hold requests only) → show_seats → holds → hold_seats / bookings / booking_seats`.
Two users asking for {10,11,12} and {13,12,11} both start with seat 11, so neither can hold a lock the other is waiting for. A test races overlapping sets to check this.

**3. Atomic, all-or-nothing hold** (`seats.AcquireHold`), all in one transaction:
1. Lock the user's row (`FOR NO KEY UPDATE`) so one user's parallel requests (two tabs) are serialised.
2. Check that the show exists and hasn't started.
3. Lock the requested seats plus the seats of the user's current hold on this show.
4. Find conflicts: an active booking, or a live hold by *another* user (`expires_at > clock_timestamp()`). If any exist, return `409 SEATS_UNAVAILABLE` with `conflictingSeatIds` / `conflictingSeats` and roll back.
5. Release the user's previous hold on this show (one hold per user per show, so the 6-seat limit can't be bypassed).
6. Delete stale rows of expired holds on those seats, then insert the new hold, expiring 5 minutes later.

**4. Expiry is evaluated at read time.** A hold counts only while `status='ACTIVE' AND expires_at > now()`, so an expired hold stops blocking seats immediately. The background cleanup (every minute, `FOR UPDATE SKIP LOCKED` so it never waits on booking transactions) only deletes stale rows. Transactions that may have waited for locks use `clock_timestamp()`, because `now()` is frozen at transaction start.

**5. Confirmation** (`bookings.Confirm`), in one transaction:
1. Read the hold.
2. Lock its seats.
3. Lock the hold row and re-validate: owner, `ACTIVE`, not expired.
4. Insert the `bookings` and `booking_seats` rows, record the payment, delete the `hold_seats` rows, and mark the hold `CONFIRMED`.

Expired → `410 HOLD_EXPIRED`. Failed payment → `402`, no booking, hold untouched. Duplicate submission → the hold is already `CONFIRMED`, so the existing booking is returned with `200`; `bookings.hold_id UNIQUE` backs this up.

**6. Database constraints as the final safeguard.**
- The partial unique index `booking_seats (show_id, seat_id) WHERE active` makes two active bookings of one seat impossible, even if a code path skipped locking. A test inserts duplicates from two raw concurrent transactions with no app code involved: exactly one gets a unique violation.
- `hold_seats UNIQUE (show_id, seat_id)` does the same for holds.
- The `shows_no_overlap` exclusion constraint does the same for scheduling.

The frontend disabling unavailable seats is only a convenience. The tests call the API and stores directly.

---

## 8. API reference

All responses are JSON. Errors look like `{"error": "human message", "code": "MACHINE_CODE", ...details}`. Every non-GET request must send `X-Requested-With: XMLHttpRequest`.

| Method & path | Auth | Description |
|---|---|---|
| `GET /api/health` | – | DB ping → `{"status":"ok"}` |
| `GET /api/config` | – | `demoLoginEnabled`, `timezone`, `holdDurationSeconds`, `maxSeatsPerBooking` |
| `POST /api/auth/signup` | – | `{name,email,password}` → 201 `{user}` + session cookie. 400 validation, 409 `EMAIL_TAKEN` |
| `POST /api/auth/login` | – | `{email,password}` → `{user}` + cookie. 401 `INVALID_CREDENTIALS` |
| `POST /api/auth/demo-login` | – | `{account: admin\|customer1\|customer2\|customer3}` → `{user}` + cookie. 404 when disabled or account missing |
| `POST /api/auth/logout` | – | 204, deletes the session |
| `GET /api/auth/me` | – | `{user}` or `{user:null}` |
| `GET /api/movies` | – | `{movies}` |
| `GET /api/movies/{id}` | – | `{movie, shows}` (upcoming only, with `remainingSeats`) |
| `GET /api/shows/{id}/seats` | optional | `{show, seats[{id,label,row,number,status}], activeHoldId, maxSeats, serverTime}` |
| `POST /api/shows/{id}/holds` | user | `{seatIds:[…]}` → 201 `{hold}`. 400 `TOO_MANY_SEATS`/`INVALID_SEAT`, 409 `SEATS_UNAVAILABLE` (+`conflictingSeats`), 410 `SHOW_STARTED` |
| `GET /api/holds/{id}` | owner | `{hold, serverTime}`; `hold.status` is `ACTIVE`/`EXPIRED`/`CONFIRMED`/`RELEASED` |
| `DELETE /api/holds/{id}` | owner | 204, releases the hold |
| `POST /api/holds/{id}/payment` | owner | `{outcome:"success"}` → 201 `{booking}` (200 on repeat). `{outcome:"failure"}` → 402 `PAYMENT_FAILED`. 410 `HOLD_EXPIRED` |
| `GET /api/bookings` | user | own bookings, upcoming first |
| `GET /api/bookings/{id}` | owner | `{booking}`; 404 for other users' bookings |
| `POST /api/bookings/{id}/cancel` | owner | 200 `{booking}`; 409 `CANCELLATION_CLOSED` / `ALREADY_CANCELLED` |
| `POST /api/admin/movies` | admin | create movie; 400 with `fields` on validation errors |
| `PUT /api/admin/movies/{id}` | admin | update (recomputes upcoming show end times; 409 `SHOW_OVERLAP`) |
| `DELETE /api/admin/movies/{id}` | admin | 204; 409 `MOVIE_HAS_BOOKINGS` |
| `GET /api/admin/shows` | admin | all shows + `occupancyPercent`, `revenuePaise`, `bookingCount` |
| `POST /api/admin/shows` | admin | `{movieId, date:"YYYY-MM-DD", startTime:"HH:MM", pricePaise}` (cinema time) → 201; 409 `SHOW_OVERLAP` (+`conflictingShow`) |
| `GET /api/admin/shows/{id}` | admin | `{show(stats), bookings(with customer), seats}` |
| `DELETE /api/admin/shows/{id}` | admin | 204; 409 `SHOW_HAS_BOOKINGS` |

Quick manual check with curl (an admin call made as a customer returns 403):

```bash
H='-H Content-Type:application/json -H X-Requested-With:XMLHttpRequest'
curl -c jar $H -d '{"account":"customer1"}' localhost:8080/api/auth/demo-login
curl -b jar -o /dev/null -w '%{http_code}\n' localhost:8080/api/admin/shows   # → 403
```

---

## 9. Tests

```bash
# Option A: in Docker (no local Go needed)
docker compose up -d db
docker compose --profile test run --rm backend-tests

# Option B: local Go, against the Compose database (host port 5433)
cd backend
TEST_DATABASE_URL="postgres://yozigo:yozigo_dev_password@localhost:5433/yozigo_test?sslmode=disable" go test -count=1 ./...
```

`yozigo_test` is a separate database, created automatically when the Postgres volume is first initialised (`backend/scripts/create-test-db.sql`). Each run drops and re-migrates its schema; each test starts from empty tables. Without `TEST_DATABASE_URL`, integration tests are **skipped** (unit tests still run).

| # | Requirement | Test (`backend/tests/`) |
|---|---|---|
| 1 | Concurrent holds on one seat → exactly one succeeds | `TestConcurrentHoldSameSeat_ExactlyOneSucceeds`: 8 goroutines × 20 rounds, released together by a barrier, separate transactions; asserts the `hold_seats` rows. Also `TestHoldWaitsForRowLock` (deterministic: B blocks while A holds the lock) and `TestConcurrentHoldOverlappingSets_NoDeadlockAllOrNothing` |
| 2 | No duplicate booking | `TestUniqueConstraintPreventsDuplicateBooking_EvenWithoutLocking` (raw concurrent transactions, app bypassed), `TestTwoUsersConfirmSameSeat_OnlyOneBooking`, `TestDuplicateConfirmIsIdempotent` |
| 3 | Expired holds become available | `TestExpiredHoldBecomesAvailable` (no cleanup job running) |
| 4 | Payment failure creates no booking | `TestPaymentFailureCreatesNoBooking` |
| 5 | Payment after expiry rejected | `TestPaymentAfterExpiryRejected` |
| 6 | Users can't access others' bookings | `TestUserCannotAccessAnotherUsersBooking` |
| 7 | Customers can't call admin endpoints | `TestCustomerCannotCallAdminEndpoints` (403 customer / 401 anonymous on all 7 routes; DB unchanged) |
| 8 | Overlapping shows rejected | `TestOverlappingShowsRejected` (edges, back-to-back allowed, IST→UTC storage, raw insert hits the constraint, duration edit) |
| 9 | Duplicate email rejected | `TestDuplicateEmailSignupRejected` (case-insensitive) |
| 10 | More than 6 seats rejected | `TestMoreThanSixSeatsRejected` |

Further tests cover demo login (allowlist, missing account, wrong role, disabled flag, idempotent seed), signup never creating admins, generic login errors, logout, CSRF, remaining-seat counts, booking order, cancellation and admin stats. Unit tests cover seat validation and labels, password rules, email normalisation and the allowlist.

The concurrency tests were checked against a deliberately broken build: removing `FOR UPDATE` makes them fail ("wins=2").

---

## 10. Local development without Docker

```bash
docker compose up -d db
cd backend && DATABASE_URL="postgres://yozigo:yozigo_dev_password@localhost:5433/yozigo?sslmode=disable" \
  DEMO_LOGIN_ENABLED=true SEED_ON_START=true COOKIE_SECURE=false go run ./cmd/api
cd frontend && npm install && npm run dev        # http://localhost:5173, proxies /api to :8080
```

## 11. Configuration

| Variable | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | – (required) | PostgreSQL connection string |
| `APP_ENV` | `development` | `production` forbids demo login |
| `DEMO_LOGIN_ENABLED` | `false` | enable the demo-login endpoint and buttons |
| `DEMO_PASSWORD` | `yozigo-demo` | password the seed gives the demo accounts |
| `SEED_ON_START` | `false` | run the idempotent seed at start-up (`.env.example` sets `true`) |
| `COOKIE_SECURE` | `true` | `Secure` cookie flag; set `false` for plain-HTTP localhost |
| `CINEMA_TIMEZONE` | `Asia/Kolkata` | zone for admin input and display |
| `PORT` | `8080` | API port |

---

## 12. Status and known limitations

**Complete:** every required flow in sections 2–10 of the brief, plus the optional features: seat polling (4 s), cancellation up to 2 h before the show with transactional seat release, and an admin dashboard that refreshes occupancy and revenue.

**Known limitations / possible next steps:**
- Payments are simulated; there's no real gateway, refunds or receipts.
- No rate limiting on login or signup, and no email verification or password reset.
- Shows can be created and deleted but not edited. There is no cleaning buffer between shows (back-to-back shows are allowed).
- A single screen with a fixed layout, by design. Multiple screens would add a `screens` table and a screen id in the exclusion constraint.
- Show deletion is blocked by *any* booking, including cancelled ones, to preserve history.
- The hold length (5 min) and seat limit (6) are constants in code, not configuration.
- There are no frontend unit tests. The UI was verified with a scripted Playwright run (not committed), and backend behaviour is covered by the Go suite.
- `COOKIE_SECURE=false` is for local HTTP only; production must run behind HTTPS.
