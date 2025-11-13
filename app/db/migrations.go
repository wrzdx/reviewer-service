package db

import (
	"context"
	"fmt"
	"log"
	"os"
)

func RunMigrations() error {
	sqlBytes, err := os.ReadFile("app/db/migrations.sql")
	if err != nil {
		return fmt.Errorf("failed to read migrations.sql: %w", err)
	}

	_, err = Pool.Exec(context.Background(), string(sqlBytes))
	if err != nil {
		log.Printf("Failed to run migrations: %v", err)
	}
	return err
}
