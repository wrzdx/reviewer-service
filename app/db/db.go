package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var Pool *pgxpool.Pool

func Init() error {
	dsn := "postgres://reviewer:reviewer@localhost:5432/reviewer_db"
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to DB: %w", err)
	}
	Pool = pool
	return nil
}

func Close() {
	if Pool != nil {
		Pool.Close()
	}
}

func RunMigrations() error {
	sql := `
	CREATE TABLE IF NOT EXISTS teams (
		team_name TEXT PRIMARY KEY
	);
	CREATE TABLE IF NOT EXISTS users (
		user_id TEXT PRIMARY KEY,
		username TEXT NOT NULL,
		team_name TEXT REFERENCES teams(team_name),
		is_active BOOLEAN NOT NULL DEFAULT true
	);
	CREATE TABLE IF NOT EXISTS pull_requests (
		pull_request_id TEXT PRIMARY KEY,
		pull_request_name TEXT NOT NULL,
		author_id TEXT REFERENCES users(user_id),
		status TEXT NOT NULL,
		assigned_reviewers TEXT[]
	);
	`
	_, err := Pool.Exec(context.Background(), sql)
	return err
}
