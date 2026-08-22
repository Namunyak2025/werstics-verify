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

const rollbackTestOrganizationID = "44444444-4444-4444-8444-444444444444"

func TestPaymentEventRollbackOnInvalidTransition(t *testing.T) {
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

	ensureRollbackOrganization(t, ctx, pool)

	repo := NewRepository(pool)

	suffix := time.Now().UnixNano()
	paymentID := fmt.Sprintf("pay_rollback_%d", suffix)
	eventID := fmt.Sprintf("evt_rollback_%d", suffix)
	providerEventID := fmt.Sprintf("provider_evt_rollback_%d", suffix)

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
		OrganizationID: rollbackTestOrganizationID,
		MerchantID:     "merchant_rollback",
		SessionID:      "session_rollback",
		Provider:       "simulator",
		ProviderRef:    "rollback-order",
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
		ProviderRef:     "rollback-order",
		MerchantID:      "merchant_rollback",
		Amount: domain.Money{
			Currency: "KES",
			Minor:    1500,
		},
		Kind:       "payment.confirmed",
		OccurredAt: now,
	}

	match := verification.Match(payment, event)

	if !match.Matched {
		t.Fatalf("expected event to match: %s", match.Reason)
	}

	_, err = repo.ApplyPaymentEvent(
		ctx,
		paymentID,
		event,
		domain.StatusConfirmed,
		match,
	)

	if err == nil {
		t.Fatal("expected invalid state transition to fail")
	}

	stored, err := repo.GetPayment(ctx, paymentID)
	if err != nil {
		t.Fatalf("get payment after rollback: %v", err)
	}

	if stored.Status != domain.StatusRequested {
		t.Fatalf(
			"expected payment to remain requested after rollback, got %s",
			stored.Status,
		)
	}

	assertCount(
		t,
		ctx,
		pool,
		"SELECT COUNT(*) FROM payment_events WHERE event_id = $1",
		[]any{eventID},
		0,
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
		0,
	)
}

func ensureRollbackOrganization(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) {
	t.Helper()

	_, err := pool.Exec(
		ctx,
		`
		INSERT INTO organizations (id, name, status)
		VALUES ($1::uuid, $2, 'active')
		ON CONFLICT (id) DO NOTHING
		`,
		rollbackTestOrganizationID,
		"Werstics Verify Rollback Tests",
	)
	if err != nil {
		t.Fatalf("create rollback test organization: %v", err)
	}
}

func TestPaymentEventRetryAfterFailedTransaction(t *testing.T) {
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

	ensureRollbackOrganization(t, ctx, pool)

	repo := NewRepository(pool)

	suffix := time.Now().UnixNano()

	paymentID := fmt.Sprintf("pay_recovery_%d", suffix)
	eventID := fmt.Sprintf("evt_recovery_%d", suffix)
	providerEventID := fmt.Sprintf("provider_evt_recovery_%d", suffix)

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
		OrganizationID: rollbackTestOrganizationID,
		MerchantID:     "merchant_recovery",
		SessionID:      "session_recovery",
		Provider:       "simulator",
		ProviderRef:    "recovery-order",
		Expected: domain.Money{
			Currency: "KES",
			Minor:    1500,
		},
		CustomerDisplay: "Recovery Test",
		Status:          domain.StatusRequested,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := repo.CreatePayment(ctx, payment); err != nil {
		t.Fatalf("create payment: %v", err)
	}

	event := domain.PaymentEvent{
		EventID:         eventID,
		Provider:        "simulator",
		ProviderEventID: providerEventID,
		PaymentID:       paymentID,
		ProviderRef:     "recovery-order",
		MerchantID:      "merchant_recovery",
		Amount: domain.Money{
			Currency: "KES",
			Minor:    1500,
		},
		CustomerDisplay: "Recovery Test",
		Kind:            "payment.confirmed",
		OccurredAt:      now.Truncate(time.Microsecond),
	}

	match := verification.Match(payment, event)

	if !match.Matched {
		t.Fatalf("expected recovery event to match: %s", match.Reason)
	}

	// First attempt intentionally fails because requested -> confirmed
	// is not yet a valid transition. The transaction must roll back.
	_, err = repo.ApplyPaymentEvent(
		ctx,
		paymentID,
		event,
		domain.StatusConfirmed,
		match,
	)
	if err == nil {
		t.Fatal("expected first processing attempt to fail")
	}

	assertCount(
		t,
		ctx,
		pool,
		"SELECT COUNT(*) FROM payment_events WHERE event_id = $1",
		[]any{eventID},
		0,
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
		0,
	)

	// Simulate the payment becoming eligible for processing after the
	// transient failure. The provider event identity remains unchanged.
	_, err = pool.Exec(
		ctx,
		`
		UPDATE payments
		SET status = 'pending',
		    updated_at = NOW()
		WHERE payment_id = $1
		`,
		paymentID,
	)
	if err != nil {
		t.Fatalf("advance payment for retry: %v", err)
	}

	retryPayment, err := repo.GetPayment(ctx, paymentID)
	if err != nil {
		t.Fatalf("reload payment for retry: %v", err)
	}

	retryMatch := verification.Match(retryPayment, event)
	if !retryMatch.Matched {
		t.Fatalf("expected retry event to match: %s", retryMatch.Reason)
	}

	updated, err := repo.ApplyPaymentEvent(
		ctx,
		paymentID,
		event,
		domain.StatusConfirmed,
		retryMatch,
	)
	if err != nil {
		t.Fatalf("retry failed: %v", err)
	}

	if updated.Status != domain.StatusConfirmed {
		t.Fatalf(
			"expected confirmed after retry, got %s",
			updated.Status,
		)
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
