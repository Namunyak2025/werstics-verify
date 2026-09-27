package api_test

import (
	"bytes"
	"context"
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
	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
	"github.com/Namunyak2025/werstics-verify/backend/internal/ingestion"
	"github.com/Namunyak2025/werstics-verify/backend/internal/payments"
	"github.com/Namunyak2025/werstics-verify/backend/internal/providers"
	"github.com/Namunyak2025/werstics-verify/backend/internal/storage/postgres"
)

func TestC2BWebhookEndToEnd(t *testing.T) {
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

	organizationID := "f0000000-0000-4000-8000-000000000002"

	ensureOrganization(
		t,
		ctx,
		pool,
		organizationID,
		"C2B Webhook Integration",
	)

	paymentID := fmt.Sprintf(
		"c2b-webhook-e2e-%d",
		time.Now().UnixNano(),
	)
	providerRef := fmt.Sprintf(
		"ORDER-C2B-%d",
		time.Now().UnixNano(),
	)

	paymentRepository := postgres.NewRepository(pool)
	paymentService := payments.NewService(paymentRepository)

	auditRepository := postgres.NewAuditRepository(pool)
	auditService := audit.NewService(auditRepository)

	registry := providers.NewRegistry(
		providers.NewC2BAdapter(),
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
			MerchantID:     "174379",
			SessionID:      "c2b-session-e2e",
			Provider:       "c2b",
			ProviderRef:    providerRef,
			Expected: domain.Money{
				Currency: "KES",
				Minor:    1500,
			},
			CustomerDisplay: "0712345678",
		},
	)
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM payments WHERE payment_id = $1",
			paymentID,
		)
	})

	payload := map[string]any{
		"TransactionType":   "Pay Bill",
		"TransID":           fmt.Sprintf("C2BTEST-%d", time.Now().UnixNano()),
		"TransTime":         "20260928123045",
		"TransAmount":       "1500.00",
		"BusinessShortCode": "174379",
		"BillRefNumber":     providerRef,
		"InvoiceNumber":     "",
		"OrgAccountBalance": "10000.00",
		"ThirdPartyTransID": "",
		"MSISDN":            "254712345678",
		"FirstName":         "Felix",
		"MiddleName":        "",
		"LastName":          "Odero",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal C2B payload: %v", err)
	}

	postConfirmation := func() *http.Response {
		req, err := http.NewRequest(
			http.MethodPost,
			httpServer.URL+"/v1/providers/c2b/webhook",
			bytes.NewReader(body),
		)
		if err != nil {
			t.Fatalf("create C2B webhook request: %v", err)
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("send C2B webhook: %v", err)
		}

		return resp
	}

	// The payment lifecycle requires requested -> pending before
	// requested -> confirmed can occur.
	pendingEvent := domain.PaymentEvent{
		EventID:         "c2b:pending-" + paymentID,
		Provider:        "c2b",
		ProviderEventID: "pending-" + paymentID,
		PaymentID:       paymentID,
		ProviderRef:     providerRef,
		MerchantID:      "174379",
		Amount: domain.Money{
			Currency: "KES",
			Minor:    1500,
		},
		CustomerDisplay: "Felix Odero",
		Kind:            "payment.pending",
		OccurredAt:      time.Now().UTC(),
	}

	if _, _, _, err := paymentService.ApplyEventDetailed(ctx, pendingEvent); err != nil {
		t.Fatalf("process pending C2B event: %v", err)
	}

	// First delivery: the C2B event must correlate to the payment
	// using BillRefNumber + BusinessShortCode.
	resp := postConfirmation()

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf(
			"expected first C2B webhook 200, got %d",
			resp.StatusCode,
		)
	}
	resp.Body.Close()

	payment, err := paymentService.Get(ctx, paymentID)
	if err != nil {
		t.Fatalf("get payment after first C2B webhook: %v", err)
	}

	if string(payment.Status) != "confirmed" {
		t.Fatalf(
			"expected confirmed payment after C2B webhook, got %q",
			payment.Status,
		)
	}

	if payment.Received.Currency != "KES" {
		t.Fatalf(
			"expected received currency KES, got %q",
			payment.Received.Currency,
		)
	}

	if payment.Received.Minor != 1500 {
		t.Fatalf(
			"expected received amount 1500 minor units, got %d",
			payment.Received.Minor,
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
		t.Fatalf("count C2B payment events: %v", err)
	}

	if eventCount != 2 {
		t.Fatalf(
			"expected pending + C2B confirmation events after first delivery, got %d",
			eventCount,
		)
	}

	// Second delivery: Safaricom can retry callbacks. The same
	// TransID must therefore be idempotent.
	resp = postConfirmation()

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf(
			"expected replayed C2B webhook 200, got %d",
			resp.StatusCode,
		)
	}
	resp.Body.Close()

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
		t.Fatalf("count C2B events after replay: %v", err)
	}

	if eventCount != 2 {
		t.Fatalf(
			"expected replay to remain idempotent with two events, got %d",
			eventCount,
		)
	}
}

func TestC2BWebhookRejectsAmountMismatch(t *testing.T) {
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

	organizationID := "f0000000-0000-4000-8000-000000000003"

	ensureOrganization(
		t,
		ctx,
		pool,
		organizationID,
		"C2B Amount Mismatch Integration",
	)

	paymentID := fmt.Sprintf(
		"c2b-mismatch-e2e-%d",
		time.Now().UnixNano(),
	)
	providerRef := fmt.Sprintf(
		"ORDER-C2B-MISMATCH-%d",
		time.Now().UnixNano(),
	)

	paymentRepository := postgres.NewRepository(pool)
	paymentService := payments.NewService(paymentRepository)

	auditRepository := postgres.NewAuditRepository(pool)
	auditService := audit.NewService(auditRepository)

	registry := providers.NewRegistry(
		providers.NewC2BAdapter(),
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
			MerchantID:     "174379",
			SessionID:      "c2b-session-mismatch",
			Provider:       "c2b",
			ProviderRef:    providerRef,
			Expected: domain.Money{
				Currency: "KES",
				Minor:    1500,
			},
			CustomerDisplay: "0712345678",
		},
	)
	if err != nil {
		t.Fatalf("create mismatch payment: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM payments WHERE payment_id = $1",
			paymentID,
		)
	})

	payload := map[string]any{
		"TransactionType":   "Pay Bill",
		"TransID":           fmt.Sprintf("C2BMISMATCH-%d", time.Now().UnixNano()),
		"TransTime":         "20260928123045",
		"TransAmount":       "2000.00",
		"BusinessShortCode": "174379",
		"BillRefNumber":     providerRef,
		"MSISDN":            "254712345678",
		"FirstName":         "Felix",
		"LastName":          "Odero",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal mismatch payload: %v", err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		httpServer.URL+"/v1/providers/c2b/webhook",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("create mismatch request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send mismatch webhook: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf(
			"expected mismatch webhook to be handled with HTTP 200, got %d",
			resp.StatusCode,
		)
	}

	payment, err := paymentService.Get(ctx, paymentID)
	if err != nil {
		t.Fatalf("get mismatch payment: %v", err)
	}

	if string(payment.Status) == "confirmed" {
		t.Fatal("amount mismatch must not confirm payment")
	}
}
