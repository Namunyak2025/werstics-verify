package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
	"github.com/Namunyak2025/werstics-verify/backend/internal/providers"
)

const (
	failureScopeOrgA = "88888888-8888-4888-8888-888888888888"
	failureScopeOrgB = "99999999-9999-4999-8999-999999999999"
)

func TestProviderEventFailureOrganizationIsolationAndResolve(t *testing.T) {
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

	for _, org := range []string{failureScopeOrgA, failureScopeOrgB} {
		_, err := pool.Exec(
			ctx,
			`
			INSERT INTO organizations (id, name, status)
			VALUES ($1::uuid, $2, 'active')
			ON CONFLICT (id) DO NOTHING
			`,
			org,
			"Failure Scope Test "+org,
		)
		if err != nil {
			t.Fatalf("create organization %s: %v", org, err)
		}
	}

	repo := NewRepository(pool)
	failures := NewProviderEventFailureRepository(pool)

	suffix := time.Now().UnixNano()

	paymentAID := fmt.Sprintf("pay-scope-a-%d", suffix)
	paymentBID := fmt.Sprintf("pay-scope-b-%d", suffix)

	now := time.Now().UTC()

	for _, payment := range []domain.Payment{
		{
			ID:             paymentAID,
			OrganizationID: failureScopeOrgA,
			MerchantID:     "merchant-a",
			SessionID:      "session-a",
			Provider:       "simulator",
			ProviderRef:    "scope-order-a",
			Expected: domain.Money{
				Currency: "KES",
				Minor:    1500,
			},
			Status:    domain.StatusRequested,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:             paymentBID,
			OrganizationID: failureScopeOrgB,
			MerchantID:     "merchant-b",
			SessionID:      "session-b",
			Provider:       "simulator",
			ProviderRef:    "scope-order-b",
			Expected: domain.Money{
				Currency: "KES",
				Minor:    2000,
			},
			Status:    domain.StatusRequested,
			CreatedAt: now,
			UpdatedAt: now,
		},
	} {
		if err := repo.CreatePayment(ctx, payment); err != nil {
			t.Fatalf("create payment %s: %v", payment.ID, err)
		}
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			`DELETE FROM payments WHERE payment_id IN ($1, $2)`,
			paymentAID,
			paymentBID,
		)
	})

	failureA := providers.EventFailure{
		Provider:        "simulator",
		ProviderEventID: fmt.Sprintf("scope-provider-a-%d", suffix),
		EventID:         fmt.Sprintf("scope-event-a-%d", suffix),
		PaymentID:       paymentAID,
		ProviderRef:     "scope-order-a",
		MerchantID:      "merchant-a",
		AmountCurrency:  "KES",
		AmountMinor:     1500,
		Kind:            "payment.confirmed",
		OccurredAt:      now,
		Status:          "retryable",
		Attempts:        1,
		LastError:       "temporary failure",
		FirstFailedAt:   now,
		LastFailedAt:    now,
	}

	failureB := providers.EventFailure{
		Provider:        "simulator",
		ProviderEventID: fmt.Sprintf("scope-provider-b-%d", suffix),
		EventID:         fmt.Sprintf("scope-event-b-%d", suffix),
		PaymentID:       paymentBID,
		ProviderRef:     "scope-order-b",
		MerchantID:      "merchant-b",
		AmountCurrency:  "KES",
		AmountMinor:     2000,
		Kind:            "payment.confirmed",
		OccurredAt:      now,
		Status:          "retryable",
		Attempts:        1,
		LastError:       "temporary failure",
		FirstFailedAt:   now,
		LastFailedAt:    now,
	}

	if err := failures.Record(ctx, failureA); err != nil {
		t.Fatalf("record failure A: %v", err)
	}

	if err := failures.Record(ctx, failureB); err != nil {
		t.Fatalf("record failure B: %v", err)
	}

	itemsA, totalA, err := failures.List(
		ctx,
		failureScopeOrgA,
		"retryable",
		1,
		100,
	)
	if err != nil {
		t.Fatalf("list organization A failures: %v", err)
	}

	if totalA < 1 || len(itemsA) < 1 {
		t.Fatalf(
			"expected at least one organization A failure, total=%d len=%d",
			totalA,
			len(itemsA),
		)
	}

	foundA := false
	for _, item := range itemsA {
		if item.PaymentID == paymentAID &&
			item.ProviderEventID == failureA.ProviderEventID {
			foundA = true
			break
		}
	}

	if !foundA {
		t.Fatalf(
			"organization A failure %q was not returned",
			failureA.ProviderEventID,
		)
	}

	// Get the database-generated failure ID for A.
	var failureAID string

	err = pool.QueryRow(
		ctx,
		`
		SELECT id::text
		FROM provider_event_failures
		WHERE provider = $1 AND provider_event_id = $2
		`,
		failureA.Provider,
		failureA.ProviderEventID,
	).Scan(&failureAID)
	if err != nil {
		t.Fatalf("get failure A id: %v", err)
	}

	// Organization B must not be able to access A's failure.
	if _, err := failures.Get(
		ctx,
		failureAID,
		failureScopeOrgB,
	); err == nil {
		t.Fatal("expected organization B to be denied access to A's failure")
	}

	if err := failures.Resolve(
		ctx,
		failureAID,
		failureScopeOrgB,
	); err == nil {
		t.Fatal("expected organization B to be denied resolving A's failure")
	}

	if err := failures.Resolve(
		ctx,
		failureAID,
		failureScopeOrgA,
	); err != nil {
		t.Fatalf("resolve organization A failure: %v", err)
	}

	resolved, err := failures.Get(
		ctx,
		failureAID,
		failureScopeOrgA,
	)
	if err != nil {
		t.Fatalf("get resolved failure: %v", err)
	}

	if resolved.Status != "resolved" {
		t.Fatalf(
			"expected resolved status, got %q",
			resolved.Status,
		)
	}

	if resolved.ResolvedAt == nil {
		t.Fatal("expected resolved_at to be populated")
	}
}
