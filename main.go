package main

import (
	"log"
	"os"

	"github.com/joho/godotenv"
	"sovereign/internal/db"
	"sovereign/internal/handlers"
)

func main() {
	// Load .env if present
	_ = godotenv.Load()

	// Connect to Postgres
	database, err := db.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("postgres connect: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// Boot router
	r := handlers.NewRouter(database)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🚀  Sovereign backend listening on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
