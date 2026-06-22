package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB = pgxpool.Pool

// Connect opens a pgx connection pool.
func Connect(dsn string) (*DB, error) {
	if dsn == "" {
		dsn = "postgres://sovereign:sovereign@localhost:5432/sovereign?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

// Migrate runs idempotent DDL.
func Migrate(db *DB) error {
	ddl := `
	CREATE TABLE IF NOT EXISTS patient_records (
		id          BIGSERIAL PRIMARY KEY,
		filename    TEXT        NOT NULL,
		raw_text    TEXT        NOT NULL,
		scribe_summary TEXT,
		created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE TABLE IF NOT EXISTS suggestions (
		id            BIGSERIAL PRIMARY KEY,
		record_id     BIGINT      NOT NULL REFERENCES patient_records(id) ON DELETE CASCADE,
		title         TEXT        NOT NULL,
		description   TEXT        NOT NULL,
		priority      TEXT        NOT NULL,
		citation      TEXT,
		created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);
	`
	_, err := db.Exec(context.Background(), ddl)
	return err
}
