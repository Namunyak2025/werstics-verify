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

	failureRepository := postgres.NewProviderEventFailureRepository(pool)

	ingestionService := ingestion.NewService(
		registry,
		paymentService,
		failureRepository,
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

func TestProviderWebhookFailurePersistsForRecovery(t *testing.T) {
	databaseURL := os.Getenv("WERSTICS_VERIFY_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("WERSTICS_VERIFY_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}

	const organizationID = "77777777-7777-4777-8777-777777777777"

	_, err = pool.Exec(
		ctx,
		`
		INSERT INTO organizations (id, name, status)
		VALUES ($1::uuid, $2, 'active')
		ON CONFLICT (id) DO NOTHING
		`,
		organizationID,
		"Werstics Verify Provider Recovery Tests",
	)
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}

	paymentRepository := postgres.NewRepository(pool)
	paymentService := payments.NewService(paymentRepository)

	now := time.Now().UTC()
	paymentID := fmt.Sprintf("pay-recovery-%d", time.Now().UnixNano())

	payment := domain.Payment{
		ID:             paymentID,
		OrganizationID: organizationID,
		MerchantID:     "merchant-recovery",
		SessionID:      "session-recovery",
		Provider:       "simulator",
		ProviderRef:    "recovery-order",
		Expected: domain.Money{
			Currency: "KES",
			Minor:    1500,
		},
		Status:    domain.StatusRequested,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := paymentRepository.CreatePayment(ctx, payment); err != nil {
		t.Fatalf("create payment: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			`DELETE FROM provider_event_failures WHERE payment_id = (
				SELECT id FROM payments WHERE payment_id = $1
			)`,
			paymentID,
		)
		_, _ = pool.Exec(
			context.Background(),
			`DELETE FROM payments WHERE payment_id = $1`,
			paymentID,
		)
	})

	// Move the payment to pending first. The next confirmed event will be
	// deliberately malformed at the persistence boundary by using a stale
	// state transition, which must roll back the payment-event transaction.
	registry := providers.NewRegistry(
		providers.NewSimulatorAdapter("recovery-test-secret"),
	)
	failureRepository := postgres.NewProviderEventFailureRepository(pool)

	ingestionService := ingestion.NewService(
		registry,
		paymentService,
		failureRepository,
	)

	pendingEvent := domain.PaymentEvent{
		EventID:         fmt.Sprintf("recovery-pending-%d", time.Now().UnixNano()),
		Provider:        "simulator",
		ProviderEventID: fmt.Sprintf("recovery-provider-pending-%d", time.Now().UnixNano()),
		PaymentID:       paymentID,
		ProviderRef:     "recovery-order",
		MerchantID:      "merchant-recovery",
		Amount: domain.Money{
			Currency: "KES",
			Minor:    1500,
		},
		Kind:       "payment.pending",
		OccurredAt: time.Now().UTC(),
	}

	adapter := providers.NewSimulatorAdapter("recovery-test-secret")
	pendingPayload, err := json.Marshal(map[string]any{
		"event_id":          pendingEvent.EventID,
		"provider_event_id": pendingEvent.ProviderEventID,
		"payment_id":        pendingEvent.PaymentID,
		"provider_ref":      pendingEvent.ProviderRef,
		"merchant_id":       pendingEvent.MerchantID,
		"amount":            pendingEvent.Amount,
		"kind":              pendingEvent.Kind,
		"occurred_at":       pendingEvent.OccurredAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("marshal pending payload: %v", err)
	}

	signature := sha256.Sum256(
		append([]byte("recovery-test-secret"), pendingPayload...),
	)

	pendingResult, err := ingestionService.IngestDetailed(
		ctx,
		"simulator",
		pendingPayload,
		map[string]string{
			"X-Werstics-Signature": hex.EncodeToString(signature[:]),
		},
	)
	if err != nil {
		t.Fatalf("pending event failed: %v", err)
	}

	if pendingResult.Payment.Status != domain.StatusPending {
		t.Fatalf("expected pending status, got %s", pendingResult.Payment.Status)
	}

	// A duplicate provider identity with altered payload must become a
	// conflict. This should not create a second payment event, while the
	// conflict itself is an operational event only when processing reaches
	// the payment repository.
	conflictPayloadMap := map[string]any{
		"event_id":          pendingEvent.EventID,
		"provider_event_id": pendingEvent.ProviderEventID,
		"payment_id":        pendingEvent.PaymentID,
		"provider_ref":      pendingEvent.ProviderRef,
		"merchant_id":       pendingEvent.MerchantID,
		"amount": map[string]any{
			"currency": "KES",
			"minor":    9999,
		},
		"kind":        pendingEvent.Kind,
		"occurred_at": pendingEvent.OccurredAt.Format(time.RFC3339Nano),
	}

	conflictPayload, err := json.Marshal(conflictPayloadMap)
	if err != nil {
		t.Fatalf("marshal conflict payload: %v", err)
	}

	conflictSignature := sha256.Sum256(
		append([]byte("recovery-test-secret"), conflictPayload...),
	)

	_, err = ingestionService.IngestDetailed(
		ctx,
		"simulator",
		conflictPayload,
		map[string]string{
			"X-Werstics-Signature": hex.EncodeToString(conflictSignature[:]),
		},
	)
	if err == nil {
		t.Fatal("expected conflict processing to fail")
	}

	var (
		failureCount int
		attempts     int
		status       string
	)

	err = pool.QueryRow(
		ctx,
		`
		SELECT COUNT(*), COALESCE(MAX(attempts), 0), COALESCE(MAX(status), '')
		FROM provider_event_failures
		WHERE payment_id = (
			SELECT id FROM payments WHERE payment_id = $1
		)
		`,
		paymentID,
	).Scan(&failureCount, &attempts, &status)
	if err != nil {
		t.Fatalf("query provider failure ledger: %v", err)
	}

	if failureCount != 1 {
		t.Fatalf("expected one provider failure, got %d", failureCount)
	}

	if attempts != 1 {
		t.Fatalf("expected one failure attempt, got %d", attempts)
	}

	if status != "retryable" {
		t.Fatalf("expected retryable failure, got %q", status)
	}

	var eventCount int
	err = pool.QueryRow(
		ctx,
		`
		SELECT COUNT(*)
		FROM payment_events
		WHERE payment_id = (
			SELECT id FROM payments WHERE payment_id = $1
		)
		`,
		paymentID,
	).Scan(&eventCount)
	if err != nil {
		t.Fatalf("count payment events: %v", err)
	}

	if eventCount != 1 {
		t.Fatalf(
			"expected only the original pending event after rollback, got %d",
			eventCount,
		)
	}

	_ = adapter
}
