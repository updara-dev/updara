package main

import (
	"crypto/rand"
	"encoding/hex"
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

	authToken := os.Getenv("UPDARA_TOKEN")
	if authToken == "" {
		authToken = s.GetSetting("auth_token")
	}
	if authToken == "" {
		b := make([]byte, 32)
		rand.Read(b)
		authToken = hex.EncodeToString(b)
		s.SetSetting("auth_token", authToken)
		log.Printf("================================================================")
		log.Printf("  UPDARA TOKEN: %s", authToken)
		log.Printf("  Set UPDARA_TOKEN env var to use a fixed token.")
		log.Printf("================================================================")
	}

	h := api.NewHandler(s, connectorsDir, binariesDir, publicURL, authToken)
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
