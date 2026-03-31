// Stockyard Trough — API cost monitor.
// Track request counts, latency, and estimated cost for any HTTP API.
// Single binary, embedded SQLite, zero external dependencies.
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/stockyard-dev/stockyard-trough/internal/server"
	"github.com/stockyard-dev/stockyard-trough/internal/store"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			fmt.Printf("trough %s\n", version)
			os.Exit(0)
		case "--health", "health":
			fmt.Println("ok")
			os.Exit(0)
		}
	}

	log.SetFlags(log.Ltime | log.Lshortfile)

	port := 8790
	if p := os.Getenv("PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}

	dataDir := "./data"
	if d := os.Getenv("DATA_DIR"); d != "" {
		dataDir = d
	}

	adminKey := os.Getenv("TROUGH_ADMIN_KEY")
	if adminKey == "" {
		log.Printf("[trough] TROUGH_ADMIN_KEY not set — admin API is open")
	}

	db, err := store.Open(dataDir)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	log.Printf("")
	log.Printf("  Stockyard Trough %s", version)
	log.Printf("  Proxy:   http://localhost:%d/proxy/{upstream_id}/...", port)
	log.Printf("  API:     http://localhost:%d/api", port)
	log.Printf("  Spend:   http://localhost:%d/api/spend", port)
	log.Printf("  Export:  http://localhost:%d/api/export.csv", port)
	log.Printf("  Health:  http://localhost:%d/health", port)
	log.Printf("")

	srv := server.New(db, port, adminKey)
	if err := srv.Start(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
