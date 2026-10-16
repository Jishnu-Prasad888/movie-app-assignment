-- 0001_init.sql — core schema for Yozigo.
--
-- Design notes (see README "Database schema" for the full explanation):
--   * Money is stored as integer paise (1 INR = 100 paise) to avoid floating point errors.
--   * All instants are TIMESTAMPTZ (stored as UTC); the cinema's local time zone is applied
--     only when parsing admin input and when formatting for display.
--   * Correctness rules that matter under concurrency are enforced by the database itself:
--       - shows_no_overlap      : exclusion constraint, the single screen can never double-book a time slot
--       - hold_seats unique     : a seat can have at most one hold row per show
--       - booking_seats partial unique index : a seat can have at most one ACTIVE booking per show
--     Application code also checks these rules (to give friendly errors), but the constraints are the
--     final safeguard if application-level locking is ever bypassed.

CREATE TABLE users (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name          TEXT        NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    email         TEXT        NOT NULL CHECK (email = lower(btrim(email))),
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'customer' CHECK (role IN ('customer', 'admin')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Emails are normalised to lower case before insert (CHECK above), so a plain unique index
-- gives case-insensitive uniqueness.
CREATE UNIQUE INDEX users_email_key ON users (email);

-- Server-side sessions. The browser only ever holds an opaque random token in an HttpOnly
-- cookie; we store SHA-256(token) so a database leak does not leak usable session tokens.
CREATE TABLE sessions (
    token_hash BYTEA       PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE movies (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title            TEXT        NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 200),
    description      TEXT        NOT NULL DEFAULT '' CHECK (length(description) <= 2000),
    language         TEXT        NOT NULL CHECK (length(btrim(language)) BETWEEN 1 AND 50),
    duration_minutes INT         NOT NULL CHECK (duration_minutes BETWEEN 1 AND 600),
    rating           TEXT        NOT NULL CHECK (rating IN ('U', 'UA', 'A')),
    poster_url       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Physical layout of the single screen: 80 seats, rows A-H, numbers 1-10.
-- Seat ids are deterministic: A1 = 1, A10 = 10, B1 = 11 ... H10 = 80.
CREATE TABLE seats (
    id          INT      PRIMARY KEY CHECK (id BETWEEN 1 AND 80),
    row_label   CHAR(1)  NOT NULL CHECK (row_label BETWEEN 'A' AND 'H'),
    seat_number SMALLINT NOT NULL CHECK (seat_number BETWEEN 1 AND 10),
    UNIQUE (row_label, seat_number)
);
INSERT INTO seats (id, row_label, seat_number)
SELECT (r - 1) * 10 + n, chr(64 + r), n
FROM generate_series(1, 8) AS r, generate_series(1, 10) AS n;

CREATE TABLE shows (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    movie_id    BIGINT      NOT NULL REFERENCES movies (id) ON DELETE RESTRICT,
    start_time  TIMESTAMPTZ NOT NULL,
    -- end_time = start_time + movie duration, computed by the application when the show is
    -- created (and recomputed if the movie's duration is edited).
    end_time    TIMESTAMPTZ NOT NULL,
    price_paise INT         NOT NULL CHECK (price_paise BETWEEN 1 AND 10000000),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (end_time > start_time),
    -- There is one screen, so no two shows may overlap in time. '[)' means a show ending at
    -- 13:00 does not conflict with one starting at 13:00. This constraint is race-proof:
    -- two admins inserting overlapping shows concurrently cannot both succeed.
    CONSTRAINT shows_no_overlap EXCLUDE USING gist (tstzrange(start_time, end_time, '[)') WITH &&)
);
CREATE INDEX shows_movie_start_idx ON shows (movie_id, start_time);
CREATE INDEX shows_start_idx ON shows (start_time);

-- One row per (show, seat). These rows exist purely so that there is always a row to lock:
-- hold, confirm and cancel transactions run SELECT ... FOR UPDATE on them in seat_id order,
-- which serialises every state change of a given seat in a given show.
CREATE TABLE show_seats (
    show_id BIGINT NOT NULL REFERENCES shows (id) ON DELETE CASCADE,
    seat_id INT    NOT NULL REFERENCES seats (id),
    PRIMARY KEY (show_id, seat_id)
);

-- Every new show automatically gets its 80 lockable seat rows, whoever inserts it.
CREATE FUNCTION create_show_seats() RETURNS trigger AS $$
BEGIN
    INSERT INTO show_seats (show_id, seat_id) SELECT NEW.id, id FROM seats;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER shows_create_seats AFTER INSERT ON shows
    FOR EACH ROW EXECUTE FUNCTION create_show_seats();

-- A hold is a temporary, per-user reservation of 1-6 seats for one show.
-- A hold is "active" only while status = 'ACTIVE' AND expires_at > now(). Expiry is therefore
-- evaluated at read time, so expired holds stop blocking seats immediately even if the
-- background cleanup has not run yet.
CREATE TABLE holds (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    show_id    BIGINT      NOT NULL REFERENCES shows (id) ON DELETE CASCADE,
    status     TEXT        NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'CONFIRMED', 'RELEASED')),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (id, show_id) -- target for the composite FK below
);
CREATE INDEX holds_user_show_active_idx ON holds (user_id, show_id) WHERE status = 'ACTIVE';
CREATE INDEX holds_expires_active_idx ON holds (expires_at) WHERE status = 'ACTIVE';

-- Seats belonging to a hold. Rows are deleted when the hold is confirmed or released, and
-- rows of expired holds are deleted lazily (by the next hold on that seat, or by cleanup).
CREATE TABLE hold_seats (
    hold_id UUID   NOT NULL,
    show_id BIGINT NOT NULL,
    seat_id INT    NOT NULL,
    PRIMARY KEY (hold_id, seat_id),
    -- the seat's show must be the hold's show
    FOREIGN KEY (hold_id, show_id) REFERENCES holds (id, show_id) ON DELETE CASCADE,
    FOREIGN KEY (show_id, seat_id) REFERENCES show_seats (show_id, seat_id) ON DELETE CASCADE,
    -- at most one hold row per seat per show
    UNIQUE (show_id, seat_id)
);

CREATE TABLE bookings (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      BIGINT      NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    show_id      BIGINT      NOT NULL REFERENCES shows (id) ON DELETE RESTRICT,
    -- One booking per hold: makes payment confirmation idempotent even under duplicate
    -- concurrent submissions.
    hold_id      UUID        UNIQUE REFERENCES holds (id) ON DELETE SET NULL,
    status       TEXT        NOT NULL CHECK (status IN ('CONFIRMED', 'CANCELLED')),
    total_paise  INT         NOT NULL CHECK (total_paise >= 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    cancelled_at TIMESTAMPTZ,
    UNIQUE (id, show_id), -- target for the composite FK below
    CHECK ((status = 'CANCELLED') = (cancelled_at IS NOT NULL))
);
CREATE INDEX bookings_user_idx ON bookings (user_id);
CREATE INDEX bookings_show_idx ON bookings (show_id);

CREATE TABLE booking_seats (
    booking_id BIGINT  NOT NULL,
    show_id    BIGINT  NOT NULL,
    seat_id    INT     NOT NULL REFERENCES seats (id),
    -- false once the booking is cancelled; the row is kept for history.
    active     BOOLEAN NOT NULL DEFAULT true,
    PRIMARY KEY (booking_id, seat_id),
    FOREIGN KEY (booking_id, show_id) REFERENCES bookings (id, show_id) ON DELETE CASCADE
);
-- THE final safeguard against double booking: a seat can be in at most one active booking
-- per show, no matter what the application does.
CREATE UNIQUE INDEX booking_seats_one_active_per_seat ON booking_seats (show_id, seat_id) WHERE active;

-- Audit trail for the fake payment gateway (both successes and failures).
CREATE TABLE payment_attempts (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    hold_id      UUID        NOT NULL REFERENCES holds (id) ON DELETE CASCADE,
    user_id      BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    outcome      TEXT        NOT NULL CHECK (outcome IN ('SUCCESS', 'FAILURE')),
    amount_paise INT         NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX payment_attempts_hold_idx ON payment_attempts (hold_id);
