package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/Namunyak2025/werstics-verify/backend/internal/api"
	"github.com/Namunyak2025/werstics-verify/backend/internal/payments"
	"github.com/Namunyak2025/werstics-verify/backend/internal/storage/postgres"
)

func main() {
	addr := os.Getenv("WERSTICS_VERIFY_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	databaseURL := os.Getenv("WERSTICS_VERIFY_DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("WERSTICS_VERIFY_DATABASE_URL is required")
	}

	ctx := context.Background()

	pool, err := postgres.NewPool(ctx, postgres.Config{
		URL: databaseURL,
	})
	if err != nil {
		log.Fatalf("database startup failed: %v", err)
	}
	defer pool.Close()

	repository := postgres.NewRepository(pool)
	paymentService := payments.NewService(repository)
	server := api.NewServer(paymentService)

	log.Printf("Werstics Verify API listening on %s", addr)

	if err := http.ListenAndServe(addr, server.Routes()); err != nil {
		log.Fatal(err)
	}
}
