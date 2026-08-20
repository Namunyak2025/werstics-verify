package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
	"github.com/Namunyak2025/werstics-verify/backend/internal/verification"
)

const idempotencyTestOrganizationID = "33333333-3333-4333-8333-333333333333"

func TestPaymentEventIdempotencyByEventID(t *testing.T) {
	databaseURL := os.Getenv("WERSTICS_VERIFY_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("WERSTICS_VERIFY_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}

	ensureOrganization(t, ctx, pool, idempotencyTestOrganizationID)

	repo := NewRepository(pool)

	suffix := time.Now().UnixNano()

	paymentID := fmt.Sprintf("pay_idempotency_%d", suffix)
	eventID := fmt.Sprintf("evt_idempotency_%d", suffix)
	providerEventID := fmt.Sprintf("provider_evt_idempotency_%d", suffix)

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM payments WHERE payment_id = $1",
			paymentID,
		)
	})

	now := time.Now().UTC()

	payment := domain.Payment{
		ID:             paymentID,
		OrganizationID: idempotencyTestOrganizationID,
		MerchantID:     "merchant_idempotency",
		SessionID:      "session_idempotency",
		Provider:       "simulator",
		ProviderRef:    "idempotency-order",
		Expected: domain.Money{
			Currency: "KES",
			Minor:    1500,
		},
		Status:    domain.StatusRequested,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := repo.CreatePayment(ctx, payment); err != nil {
		t.Fatalf("create payment: %v", err)
	}

	event := domain.PaymentEvent{
		EventID:         eventID,
		Provider:        "simulator",
		ProviderEventID: providerEventID,
		PaymentID:       paymentID,
		ProviderRef:     "idempotency-order",
		MerchantID:      "merchant_idempotency",
		Amount: domain.Money{
			Currency: "KES",
			Minor:    1500,
		},
		Kind:       "payment.pending",
		OccurredAt: now,
	}

	match := verification.Match(payment, event)

	first, err := repo.ApplyPaymentEvent(
		ctx,
		paymentID,
		event,
		domain.StatusPending,
		match,
	)
	if err != nil {
		t.Fatalf("first event: %v", err)
	}

	if first.Status != domain.StatusPending {
		t.Fatalf("expected pending, got %s", first.Status)
	}

	second, err := repo.ApplyPaymentEvent(
		ctx,
		paymentID,
		event,
		domain.StatusPending,
		match,
	)
	if err != nil {
		t.Fatalf("duplicate event: %v", err)
	}

	if second.Status != domain.StatusPending {
		t.Fatalf("expected duplicate to preserve pending, got %s", second.Status)
	}

	assertCount(
		t,
		ctx,
		pool,
		"SELECT COUNT(*) FROM payment_events WHERE event_id = $1",
		[]any{eventID},
		1,
	)

	assertCount(
		t,
		ctx,
		pool,
		`
		SELECT COUNT(*)
		FROM payment_verifications
		WHERE event_id = (
			SELECT id
			FROM payment_events
			WHERE event_id = $1
		)
		`,
		[]any{eventID},
		1,
	)
}

func TestPaymentEventIdempotencyByProviderEventID(t *testing.T) {
	databaseURL := os.Getenv("WERSTICS_VERIFY_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("WERSTICS_VERIFY_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}

	ensureOrganization(t, ctx, pool, idempotencyTestOrganizationID)

	repo := NewRepository(pool)

	suffix := time.Now().UnixNano()

	paymentID := fmt.Sprintf("pay_provider_idempotency_%d", suffix)
	firstEventID := fmt.Sprintf("evt_provider_a_%d", suffix)
	secondEventID := fmt.Sprintf("evt_provider_b_%d", suffix)
	providerEventID := fmt.Sprintf("provider_duplicate_%d", suffix)

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM payments WHERE payment_id = $1",
			paymentID,
		)
	})

	now := time.Now().UTC()

	payment := domain.Payment{
		ID:             paymentID,
		OrganizationID: idempotencyTestOrganizationID,
		MerchantID:     "merchant_provider_idempotency",
		SessionID:      "session_provider_idempotency",
		Provider:       "simulator",
		ProviderRef:    "provider-idempotency-order",
		Expected: domain.Money{
			Currency: "KES",
			Minor:    1500,
		},
		Status:    domain.StatusRequested,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := repo.CreatePayment(ctx, payment); err != nil {
		t.Fatalf("create payment: %v", err)
	}

	firstEvent := domain.PaymentEvent{
		EventID:         firstEventID,
		Provider:        "simulator",
		ProviderEventID: providerEventID,
		PaymentID:       paymentID,
		ProviderRef:     "provider-idempotency-order",
		MerchantID:      "merchant_provider_idempotency",
		Amount: domain.Money{
			Currency: "KES",
			Minor:    1500,
		},
		Kind:       "payment.pending",
		OccurredAt: now,
	}

	match := verification.Match(payment, firstEvent)

	if _, err := repo.ApplyPaymentEvent(
		ctx,
		paymentID,
		firstEvent,
		domain.StatusPending,
		match,
	); err != nil {
		t.Fatalf("first event: %v", err)
	}

	secondEvent := firstEvent
	secondEvent.EventID = secondEventID

	second, err := repo.ApplyPaymentEvent(
		ctx,
		paymentID,
		secondEvent,
		domain.StatusPending,
		match,
	)
	if err != nil {
		t.Fatalf("provider duplicate: %v", err)
	}

	if second.Status != domain.StatusPending {
		t.Fatalf(
			"expected provider duplicate to preserve pending, got %s",
			second.Status,
		)
	}

	assertCount(
		t,
		ctx,
		pool,
		"SELECT COUNT(*) FROM payment_events WHERE provider = $1 AND provider_event_id = $2",
		[]any{"simulator", providerEventID},
		1,
	)
}

func ensureOrganization(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	organizationID string,
) {
	t.Helper()

	_, err := pool.Exec(
		ctx,
		`
		INSERT INTO organizations (id, name, status)
		VALUES ($1::uuid, $2, 'active')
		ON CONFLICT (id) DO NOTHING
		`,
		organizationID,
		"Werstics Verify Idempotency Tests",
	)
	if err != nil {
		t.Fatalf("create test organization: %v", err)
	}
}

func assertCount(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	query string,
	args []any,
	expected int,
) {
	t.Helper()

	var count int

	if err := pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}

	if count != expected {
		t.Fatalf("expected count %d, got %d", expected, count)
	}
}
