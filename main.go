/**
 * @file main.go
 * @brief Application entry point.
 *
 * Reads configuration from environment variables, opens the database,
 * and starts the HTTP server.  All configuration has a safe default so
 * the binary can be run locally with no environment setup.
 *
 * Environment variables:
 *   DATABASE_URL  — SQLite file path            (default: tasks.db)
 *   JWT_SECRET    — HMAC secret for JWT signing (default: dev-secret-change-in-production)
 *   ADDR          — TCP address to listen on    (default: :8080)
 */
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/jcarp/goserver/internal/db"
	"github.com/jcarp/goserver/internal/server"
)

/**
 * @brief Starts the Task API server.
 *
 * Initialisation order:
 *   1. Read config from environment (with fallbacks).
 *   2. Open SQLite database and apply schema migrations.
 *   3. Build the composed HTTP handler via server.New.
 *   4. Start listening; log.Fatalf on any error.
 */
func main() {
	dsn := getEnv("DATABASE_URL", "tasks.db")
	jwtSecret := getEnv("JWT_SECRET", "dev-secret-change-in-production")
	addr := getEnv("ADDR", ":8080")

	database, err := db.Open(dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	srv := server.New(database, jwtSecret)
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, srv); err != nil {
		log.Fatalf("server: %v", err)
	}
}

/**
 * @brief Returns the value of an environment variable, or a fallback string.
 *
 * @param key       The environment variable name to look up.
 * @param fallback  The value to return when the variable is unset or empty.
 * @return          The environment variable value, or fallback.
 */
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
