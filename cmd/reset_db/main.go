package main

import (
	"fmt"
	"log"
	"os"
	"fresh-words-backend/config"
	"fresh-words-backend/db"
)

func main() {
	os.Setenv("SKIP_MIGRATION", "true")
	config.LoadConfig()
	db.ConnectDB()

	fmt.Println("Dropping all tables to reset the database...")

	tables := []string{
		"devotional_reads",
		"user_bookmarks",
		"devotional_schedules",
		"devotionals",
		"packages",
		"feedbacks",
		"settings",
		"users",
	}

	for _, table := range tables {
		err := db.DB.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", table)).Error
		if err != nil {
			log.Fatalf("Failed to drop table %s: %v", table, err)
		}
		fmt.Printf("Dropped table %s successfully.\n", table)
	}

	fmt.Println("Database tables dropped completely! Start the server to automatically run migrations and bootstrap the admin account.")
}
