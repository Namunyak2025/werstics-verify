package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Namunyak2025/werstics-verify/backend/internal/api"
	"github.com/Namunyak2025/werstics-verify/backend/internal/audit"
	"github.com/Namunyak2025/werstics-verify/backend/internal/auth"
	"github.com/Namunyak2025/werstics-verify/backend/internal/config"
	"github.com/Namunyak2025/werstics-verify/backend/internal/ingestion"
	"github.com/Namunyak2025/werstics-verify/backend/internal/payments"
	"github.com/Namunyak2025/werstics-verify/backend/internal/providers"
	"github.com/Namunyak2025/werstics-verify/backend/internal/storage/postgres"
)

const shutdownTimeout = 15 * time.Second

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuration validation failed: %v", err)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	pool, err := postgres.NewPool(ctx, postgres.Config{
		URL: cfg.DatabaseURL,
	})
	if err != nil {
		log.Fatalf("database startup failed: %v", err)
	}
	defer pool.Close()

	paymentRepository := postgres.NewRepository(pool)
	paymentService := payments.NewService(paymentRepository)

	authRepository := postgres.NewAuthRepository(pool)
	authService := auth.NewService(authRepository)

	rbacRepository := postgres.NewRBACRepository(pool)

	auditRepository := postgres.NewAuditRepository(pool)
	auditService := audit.NewService(auditRepository)

	providerRegistry := providers.NewRegistry(
		providers.NewSimulatorAdapter(cfg.SimulatorSecret),
	)

	failureRepository := postgres.NewProviderEventFailureRepository(pool)

	ingestionService := ingestion.NewService(
		providerRegistry,
		paymentService,
		failureRepository,
	)

	server := api.NewServer(
		paymentService,
		authService,
		rbacRepository,
		auditService,
	)
	server.SetReadinessChecker(pool)
	server.SetProviderIngestion(ingestionService)
	server.SetProviderFailureRepository(failureRepository)

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)

	go func() {
		log.Printf("Werstics Verify API listening on %s", cfg.Addr)

		if err := httpServer.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		log.Fatalf("HTTP server failed: %v", err)

	case <-ctx.Done():
		log.Printf("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		shutdownTimeout,
	)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("HTTP server shutdown failed: %v", err)
	}

	log.Printf("Werstics Verify API stopped")
}
