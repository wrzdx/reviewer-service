package main

import (
	"log"
	"reviewer-service/app/db"
)

func main() {
	// Инициализируем соединение
	if err := db.Init(); err != nil {
		log.Fatal("Failed to init DB:", err)
	}
	defer db.Close()

	// Применяем миграции
	if err := db.RunMigrations(); err != nil {
		log.Fatal("Migration failed:", err)
	}

	log.Println("Migrations applied successfully")
}
