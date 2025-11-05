package main

import (
	"flag"
	"fmt"
	"kv-server/internal/config"
	"kv-server/internal/database"
	"kv-server/internal/server"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	// Load environment variables from .env file
	if err := config.LoadEnv(".env"); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}

	// Command-line flags with env variable defaults
	port := flag.Int("port", getEnvAsInt("SERVER_PORT", 8080), "Server port")
	workers := flag.Int("workers", getEnvAsInt("WORKER_THREADS", 10), "Number of worker threads")
	cacheSize := flag.Int("cache-size", getEnvAsInt("CACHE_SIZE", 1000), "Cache capacity")

	dbHost := flag.String("db-host", config.GetEnv("DB_HOST", "localhost"), "Database host")
	dbPort := flag.String("db-port", config.GetEnv("DB_PORT", "5432"), "Database port")
	dbUser := flag.String("db-user", config.GetEnv("DB_USER", "postgres"), "Database user")
	dbPass := flag.String("db-pass", config.GetEnv("DB_PASSWORD", "postgres"), "Database password")
	dbName := flag.String("db-name", config.GetEnv("DB_NAME", "kvstore"), "Database name")

	flag.Parse()

	// Connect to database
	db, err := database.NewPostgresDB(*dbHost, *dbPort, *dbUser, *dbPass, *dbName)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	log.Printf("Connected to PostgreSQL database at %s:%s", *dbHost, *dbPort)

	// Create KV server
	kvServer := server.NewKVServer(*cacheSize, db)

	// Configure HTTP server with thread pool
	httpServer := &http.Server{
		Addr:           fmt.Sprintf(":%d", *port),
		Handler:        kvServer,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	// Start stats printer
	go printStats(kvServer)

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		log.Println("\nShutting down server...")
		os.Exit(0)
	}()

	log.Printf("Server starting on port %d with %d workers and cache size %d", *port, *workers, *cacheSize)
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func printStats(kvServer *server.KVServer) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		hits, misses := kvServer.GetCacheStats()
		total := hits + misses
		hitRate := float64(0)
		if total > 0 {
			hitRate = float64(hits) / float64(total) * 100
		}
		log.Printf("Cache Stats - Hits: %d, Misses: %d, Hit Rate: %.2f%%", hits, misses, hitRate)
	}
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}
