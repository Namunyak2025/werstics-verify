package postgres

import (
	"context"
	"fmt"
	"os"
	"strings"
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

func TestPaymentEventRejectsEventIDForDifferentPayment(t *testing.T) {
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

	paymentAID := fmt.Sprintf("pay_conflict_a_%d", suffix)
	paymentBID := fmt.Sprintf("pay_conflict_b_%d", suffix)
	eventID := fmt.Sprintf("evt_conflict_payment_%d", suffix)
	providerEventIDA := fmt.Sprintf("provider_conflict_payment_%d", suffix)

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM payments WHERE payment_id IN ($1, $2)",
			paymentAID,
			paymentBID,
		)
	})

	now := time.Now().UTC()

	paymentA := domain.Payment{
		ID:             paymentAID,
		OrganizationID: idempotencyTestOrganizationID,
		MerchantID:     "merchant_conflict_a",
		SessionID:      "session_conflict_a",
		Provider:       "simulator",
		ProviderRef:    "conflict-order-a",
		Expected:       domain.Money{Currency: "KES", Minor: 1500},
		Status:         domain.StatusRequested,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	paymentB := paymentA
	paymentB.ID = paymentBID
	paymentB.MerchantID = "merchant_conflict_b"
	paymentB.SessionID = "session_conflict_b"
	paymentB.ProviderRef = "conflict-order-b"

	if err := repo.CreatePayment(ctx, paymentA); err != nil {
		t.Fatalf("create payment A: %v", err)
	}

	if err := repo.CreatePayment(ctx, paymentB); err != nil {
		t.Fatalf("create payment B: %v", err)
	}

	event := domain.PaymentEvent{
		EventID:         eventID,
		Provider:        "simulator",
		ProviderEventID: providerEventIDA,
		PaymentID:       paymentAID,
		ProviderRef:     "conflict-order-a",
		MerchantID:      "merchant_conflict_a",
		Amount:          domain.Money{Currency: "KES", Minor: 1500},
		Kind:            "payment.pending",
		OccurredAt:      now,
	}

	match := verification.Match(paymentA, event)

	if _, err := repo.ApplyPaymentEvent(
		ctx,
		paymentAID,
		event,
		domain.StatusPending,
		match,
	); err != nil {
		t.Fatalf("first event: %v", err)
	}

	event.PaymentID = paymentBID
	event.MerchantID = paymentB.MerchantID
	event.ProviderRef = paymentB.ProviderRef

	match = verification.Match(paymentB, event)

	_, err = repo.ApplyPaymentEvent(
		ctx,
		paymentBID,
		event,
		domain.StatusPending,
		match,
	)
	if err == nil {
		t.Fatal("expected event ID reuse across payments to be rejected")
	}

	if !strings.Contains(err.Error(), "conflicting payment event") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPaymentEventRejectsAlteredProviderPayload(t *testing.T) {
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

	paymentID := fmt.Sprintf("pay_conflict_payload_%d", suffix)
	eventID := fmt.Sprintf("evt_conflict_payload_%d", suffix)
	providerEventID := fmt.Sprintf("provider_conflict_payload_%d", suffix)

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
		MerchantID:     "merchant_conflict_payload",
		SessionID:      "session_conflict_payload",
		Provider:       "simulator",
		ProviderRef:    "conflict-payload-order",
		Expected:       domain.Money{Currency: "KES", Minor: 1500},
		Status:         domain.StatusRequested,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := repo.CreatePayment(ctx, payment); err != nil {
		t.Fatalf("create payment: %v", err)
	}

	event := domain.PaymentEvent{
		EventID:         eventID,
		Provider:        "simulator",
		ProviderEventID: providerEventID,
		PaymentID:       paymentID,
		ProviderRef:     "conflict-payload-order",
		MerchantID:      payment.MerchantID,
		Amount:          domain.Money{Currency: "KES", Minor: 1500},
		Kind:            "payment.pending",
		OccurredAt:      now,
	}

	match := verification.Match(payment, event)

	if _, err := repo.ApplyPaymentEvent(
		ctx,
		paymentID,
		event,
		domain.StatusPending,
		match,
	); err != nil {
		t.Fatalf("first event: %v", err)
	}

	altered := event
	altered.EventID = fmt.Sprintf("evt_conflict_payload_altered_%d", suffix)
	altered.Amount.Minor = 1700

	alteredMatch := verification.Match(payment, altered)

	_, err = repo.ApplyPaymentEvent(
		ctx,
		paymentID,
		altered,
		domain.StatusPending,
		alteredMatch,
	)
	if err == nil {
		t.Fatal("expected altered provider payload to be rejected")
	}

	if !strings.Contains(err.Error(), "conflicting payment event") {
		t.Fatalf("unexpected error: %v", err)
	}
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
