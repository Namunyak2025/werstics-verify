package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
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

func spaHandler(
	apiHandler http.Handler,
	distDir string,
) http.Handler {
	fileServer := http.FileServer(http.Dir(distDir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1" ||
			strings.HasPrefix(r.URL.Path, "/v1/") ||
			r.URL.Path == "/health" ||
			strings.HasPrefix(r.URL.Path, "/health/") {
			apiHandler.ServeHTTP(w, r)
			return
		}

		requestPath := strings.TrimPrefix(r.URL.Path, "/")
		cleanPath := filepath.Clean(requestPath)

		if cleanPath != "." &&
			cleanPath != ".." &&
			!strings.HasPrefix(cleanPath, ".."+string(os.PathSeparator)) {
			filePath := filepath.Join(distDir, cleanPath)

			if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		indexPath := filepath.Join(distDir, "index.html")

		if _, err := os.Stat(indexPath); err != nil {
			http.Error(
				w,
				"frontend assets unavailable",
				http.StatusInternalServerError,
			)
			return
		}

		http.ServeFile(w, r, indexPath)
	})
}

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

	apiHandler := server.Routes()

	httpServer := &http.Server{
		Addr: cfg.Addr,
		Handler: spaHandler(
			apiHandler,
			"web/dist",
		),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)

	go func() {
		log.Printf(
			"Werstics Verify server listening on %s",
			cfg.Addr,
		)

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

	log.Printf("Werstics Verify server stopped")
}
