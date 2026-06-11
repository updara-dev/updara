package main

import (
	"log"
	"net/http"
	"os"

	"github.com/updara/server/api"
	"github.com/updara/server/store"
)

func main() {
	dbPath := env("DB_PATH", "/data/updara.json")
	connectorsDir := env("CONNECTORS_DIR", "/app/connectors")
	binariesDir := env("BINARIES_DIR", "/app/binaries")
	publicURL := os.Getenv("PUBLIC_URL")
	addr := env("LISTEN_ADDR", ":8080")

	s := store.New(dbPath)
	h := api.NewHandler(s, connectorsDir, binariesDir, publicURL)
	h.StartNotificationScheduler()

	log.Printf("Updara server listening on %s", addr)
	if err := http.ListenAndServe(addr, h.Router()); err != nil {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
