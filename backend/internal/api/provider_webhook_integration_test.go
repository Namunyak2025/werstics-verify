package api_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Namunyak2025/werstics-verify/backend/internal/api"
	"github.com/Namunyak2025/werstics-verify/backend/internal/audit"
	"github.com/Namunyak2025/werstics-verify/backend/internal/auth"
	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
	"github.com/Namunyak2025/werstics-verify/backend/internal/ingestion"
	"github.com/Namunyak2025/werstics-verify/backend/internal/payments"
	"github.com/Namunyak2025/werstics-verify/backend/internal/providers"
	"github.com/Namunyak2025/werstics-verify/backend/internal/storage/postgres"
)

func TestProviderWebhookEndToEnd(t *testing.T) {
	databaseURL := os.Getenv("WERSTICS_VERIFY_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("WERSTICS_VERIFY_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		20*time.Second,
	)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}

	organizationID := "f0000000-0000-4000-8000-000000000001"
	ensureOrganization(
		t,
		ctx,
		pool,
		organizationID,
		"Webhook Integration",
	)

	paymentID := fmt.Sprintf(
		"webhook-e2e-%d",
		time.Now().UnixNano(),
	)

	paymentRepository := postgres.NewRepository(pool)
	paymentService := payments.NewService(paymentRepository)

	authRepository := postgres.NewAuthRepository(pool)
	authService := auth.NewService(authRepository)

	rbacRepository := postgres.NewRBACRepository(pool)

	auditRepository := postgres.NewAuditRepository(pool)
	auditService := audit.NewService(auditRepository)

	_ = authService
	_ = rbacRepository

	secret := "webhook-integration-secret"

	registry := providers.NewRegistry(
		providers.NewSimulatorAdapter(secret),
	)

	ingestionService := ingestion.NewService(
		registry,
		paymentService,
	)

	server := api.NewServer(
		paymentService,
		nil,
		nil,
		auditService,
	)

	server.SetProviderIngestion(ingestionService)

	httpServer := httptest.NewServer(server.Routes())
	defer httpServer.Close()

	err = paymentService.Create(
		ctx,
		domain.Payment{
			ID:             paymentID,
			OrganizationID: organizationID,
			MerchantID:     "merchant-e2e",
			SessionID:      "session-e2e",
			Provider:       "simulator",
			ProviderRef:    "ORDER-E2E-001",
			Expected: domain.Money{
				Currency: "KES",
				Minor:    1500,
			},
			CustomerDisplay: "E2E Customer",
		},
	)
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}

	postWebhook := func(
		kind string,
		eventSuffix string,
	) {
		payload := map[string]any{
			"event_id":          fmt.Sprintf("evt-%s-%d", eventSuffix, time.Now().UnixNano()),
			"provider_event_id": fmt.Sprintf("provider-evt-%s-%d", eventSuffix, time.Now().UnixNano()),
			"payment_id":        paymentID,
			"provider_ref":      "ORDER-E2E-001",
			"merchant_id":       "merchant-e2e",
			"amount": map[string]any{
				"currency": "KES",
				"minor":    1500,
			},
			"customer_display": "E2E Customer",
			"kind":             kind,
			"occurred_at":      time.Now().UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano),
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal %s payload: %v", kind, err)
		}

		sum := sha256.Sum256(
			append([]byte(secret), body...),
		)
		signature := hex.EncodeToString(sum[:])

		req, err := http.NewRequest(
			http.MethodPost,
			httpServer.URL+"/v1/providers/simulator/webhook",
			bytes.NewReader(body),
		)
		if err != nil {
			t.Fatalf("create %s webhook request: %v", kind, err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Werstics-Signature", signature)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("send %s webhook: %v", kind, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf(
				"expected %s webhook 200, got %d",
				kind,
				resp.StatusCode,
			)
		}
	}

	postWebhook("payment.pending", "pending")
	postWebhook("payment.confirmed", "confirmed")

	payment, err := paymentService.Get(ctx, paymentID)
	if err != nil {
		t.Fatalf("get payment after webhook: %v", err)
	}

	if string(payment.Status) != "confirmed" {
		t.Fatalf(
			"expected confirmed payment, got %q",
			payment.Status,
		)
	}

	var eventCount int

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*)
		 FROM payment_events
		 WHERE payment_id = (
		     SELECT id FROM payments WHERE payment_id = $1
		 )`,
		paymentID,
	).Scan(&eventCount)
	if err != nil {
		t.Fatalf("count payment events: %v", err)
	}

	if eventCount != 2 {
		t.Fatalf(
			"expected two payment events, got %d",
			eventCount,
		)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM payments WHERE payment_id = $1",
			paymentID,
		)
	})
}
