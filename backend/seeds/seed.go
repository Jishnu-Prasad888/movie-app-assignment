// Package seeds inserts deterministic demo data: the four demo accounts, a small movie
// catalogue and a week of shows. It is idempotent and safe to run repeatedly:
//   - demo users are upserted by email (existing rows are left alone);
//   - movies are inserted only if the movies table is empty;
//   - shows are inserted only if the shows table is empty.
package seeds

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"yozigo/internal/auth"
	"yozigo/internal/shows"
)

type movieSeed struct {
	title, language, rating, description string
	duration                             int
}

// Fictional titles. Every duration is < 3h30, the gap between the daily show slots.
var movieSeeds = []movieSeed{
	{"The Monsoon Line", "Hindi", "UA", "A railway engineer races a rising flood to bring the last train of the season home.", 148},
	{"Neon Harbor", "English", "A", "A burned-out detective follows a missing shipment through a rain-soaked port city.", 122},
	{"Vaanam Thedi", "Tamil", "U", "A village girl builds a rocket from scrap and the whole town gets swept up in her dream.", 156},
	{"Paper Kites", "English", "U", "An animated adventure about two siblings who follow a runaway kite across the country.", 98},
	{"Backwater Blues", "Malayalam", "UA", "A houseboat musician and a city chef cross paths during one unforgettable monsoon.", 135},
	{"Signal Lost", "Telugu", "UA", "When every phone in Hyderabad goes silent, a radio hobbyist becomes the city's only link.", 141},
}

// Daily slots in the cinema's local time and their ticket prices (paise).
var slots = []struct {
	hour, minute int
	pricePaise   int
}{
	{10, 0, 18000},
	{13, 30, 22000},
	{17, 0, 25000},
	{20, 30, 30000},
}

const seedDays = 7

func Run(ctx context.Context, pool *pgxpool.Pool, demoPassword string, loc *time.Location) error {
	hash, err := auth.HashPassword(demoPassword)
	if err != nil {
		return err
	}
	for _, a := range auth.DemoAccounts {
		if _, err := pool.Exec(ctx, `
			INSERT INTO users (name, email, password_hash, role) VALUES ($1, $2, $3, $4)
			ON CONFLICT (email) DO NOTHING`, a.Name, a.Email, hash, a.Role); err != nil {
			return fmt.Errorf("seed user %s: %w", a.Key, err)
		}
	}

	var movieCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM movies`).Scan(&movieCount); err != nil {
		return err
	}
	if movieCount == 0 {
		for _, m := range movieSeeds {
			if _, err := pool.Exec(ctx, `
				INSERT INTO movies (title, description, language, duration_minutes, rating)
				VALUES ($1, $2, $3, $4, $5)`, m.title, m.description, m.language, m.duration, m.rating); err != nil {
				return fmt.Errorf("seed movie %q: %w", m.title, err)
			}
		}
	}

	var showCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM shows`).Scan(&showCount); err != nil {
		return err
	}
	if showCount == 0 {
		rows, err := pool.Query(ctx, `SELECT id FROM movies ORDER BY id`)
		if err != nil {
			return err
		}
		var movieIDs []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			movieIDs = append(movieIDs, id)
		}
		rows.Close()
		if len(movieIDs) > 0 {
			showStore := shows.NewStore(pool)
			now := time.Now().In(loc)
			today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
			n := 0
			for d := 0; d < seedDays; d++ {
				for i, sl := range slots {
					start := time.Date(today.Year(), today.Month(), today.Day()+d, sl.hour, sl.minute, 0, 0, loc)
					if !start.After(now) {
						continue // never seed shows in the past
					}
					movieID := movieIDs[(d*len(slots)+i)%len(movieIDs)]
					if _, err := showStore.Create(ctx, movieID, start, sl.pricePaise); err != nil {
						return fmt.Errorf("seed show %s: %w", start, err)
					}
					n++
				}
			}
			slog.Info("seeded shows", "count", n)
		}
	}
	slog.Info("seed complete", "demo_accounts", len(auth.DemoAccounts))
	return nil
}
