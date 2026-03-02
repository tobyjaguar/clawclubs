package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/tobyjaguar/clawclubs/internal/server"
	"github.com/tobyjaguar/clawclubs/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", "clawclubs.db", "SQLite database path")
	flag.Parse()

	adminKey := os.Getenv("CLAWCLUBS_ADMIN_KEY")
	if adminKey == "" {
		adminKey = "changeme"
		log.Println("WARNING: using default admin key. Set CLAWCLUBS_ADMIN_KEY in production.")
	}

	st, err := store.New(*dbPath)
	if err != nil {
		log.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	srv := server.New(st, adminKey)

	log.Printf("ClawClubs listening on %s", *addr)
	if err := http.ListenAndServe(*addr, srv); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
