// app/db/db_integration_test.go
package db_test

import (
	"context"
	"testing"

	"reviewer-service/app/db"
	"reviewer-service/app/testutils"
)

func setupDB(t *testing.T) {
	if err := testutils.LoadTestEnv("../../.env.test"); err != nil {
		t.Fatalf("failed to load env: %v", err)
	}
	if err := db.Init(); err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
}

func TestDBConnection(t *testing.T) {
	setupDB(t)

	var one int
	err := db.Pool.QueryRow(context.Background(), "SELECT 1").Scan(&one)
	if err != nil || one != 1 {
		t.Fatalf("expected 1, got %v, err=%v", one, err)
	}
}

func TestTableExists(t *testing.T) {
	setupDB(t)

	var count int
	err := db.Pool.QueryRow(context.Background(),
		"SELECT count(*) FROM information_schema.tables WHERE table_schema='public'").Scan(&count)
	if err != nil || count == 0 {
		t.Fatalf("expected tables to exist, got count=%d, err=%v", count, err)
	}
}

func TestInsertAndSelectPR(t *testing.T) {
	setupDB(t)

	ctx := context.Background()

	// Вставляем команду
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO teams (team_name) VALUES ($1)
	`, "backend")
	if err != nil {
		t.Fatalf("failed to insert team: %v", err)
	}

	// Вставляем пользователя
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO users (user_id, username, team_name, is_active) VALUES ($1, $2, $3, $4)
	`, "u1", "Alice", "backend", true)
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}

	// Вставляем PR
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO pull_requests (pull_request_id, pull_request_name, author_id, status, assigned_reviewers)
		VALUES ($1, $2, $3, $4, $5)
	`, "pr-test", "Test PR", "u1", "OPEN", []string{})
	if err != nil {
		t.Fatalf("failed to insert PR: %v", err)
	}

	// Проверяем PR
	var status string
	err = db.Pool.QueryRow(ctx,
		"SELECT status FROM pull_requests WHERE pull_request_id=$1", "pr-test").Scan(&status)
	if err != nil {
		t.Fatalf("failed to select PR: %v", err)
	}

	if status != "OPEN" {
		t.Fatalf("expected status OPEN, got %s", status)
	}
}
