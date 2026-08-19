package main

import (
	"log"
	"net/http"
	"os"

	"github.com/Namunyak2025/werstics-verify/backend/internal/api"
)

func main() {
	addr := os.Getenv("WERSTICS_VERIFY_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	server := api.NewServer()
	log.Printf("Werstics Verify API listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, server.Routes()))
}
