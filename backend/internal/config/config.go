// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // embed the IANA tz database so time zones work in minimal containers
)

type Config struct {
	Port        string
	DatabaseURL string
	AppEnv      string // "development" or "production"

	// DemoLoginEnabled turns on POST /api/auth/demo-login. Defaults to false and is refused
	// outright when AppEnv is "production".
	DemoLoginEnabled bool
	// DemoPassword is the password the seed script gives the four demo accounts, so they can
	// also be used with the normal login form. Only used by the seeder, never sent to clients.
	DemoPassword string
	SeedOnStart  bool

	CookieSecure bool
	SessionTTL   time.Duration
	HoldDuration time.Duration

	// The cinema's local time zone. Admin input ("2026-10-20 18:30") is interpreted in it and
	// the frontend formats times in it; the database stores UTC instants.
	CinemaTimezone *time.Location
}

func Load() (Config, error) {
	cfg := Config{
		Port:             getenv("PORT", "8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		AppEnv:           getenv("APP_ENV", "development"),
		DemoLoginEnabled: getbool("DEMO_LOGIN_ENABLED", false),
		DemoPassword:     getenv("DEMO_PASSWORD", "yozigo-demo"),
		SeedOnStart:      getbool("SEED_ON_START", false),
		CookieSecure:     getbool("COOKIE_SECURE", true),
		SessionTTL:       24 * time.Hour,
		HoldDuration:     5 * time.Minute,
	}
	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.AppEnv == "production" && cfg.DemoLoginEnabled {
		return cfg, fmt.Errorf("DEMO_LOGIN_ENABLED=true is not allowed when APP_ENV=production")
	}
	tzName := getenv("CINEMA_TIMEZONE", "Asia/Kolkata")
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return cfg, fmt.Errorf("invalid CINEMA_TIMEZONE %q: %w", tzName, err)
	}
	cfg.CinemaTimezone = loc
	return cfg, nil
}

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func getbool(key string, def bool) bool {
	v, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(key)))
	if err != nil {
		return def
	}
	return v
}
