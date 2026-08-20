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

const integrationOrganizationID = "22222222-2222-4222-8222-222222222222"

func TestPostgresRepositoryIntegration(t *testing.T) {
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

	_, err = pool.Exec(
		ctx,
		`
		INSERT INTO organizations (id, name, status)
		VALUES ($1::uuid, $2, 'active')
		ON CONFLICT (id) DO NOTHING
		`,
		integrationOrganizationID,
		"Werstics Verify Integration Tests",
	)
	if err != nil {
		t.Fatalf("create integration organization: %v", err)
	}

	repo := NewRepository(pool)

	paymentID := fmt.Sprintf(
		"pay_integration_%d",
		time.Now().UnixNano(),
	)

	eventID := fmt.Sprintf(
		"evt_integration_%d",
		time.Now().UnixNano(),
	)

	providerEventID := fmt.Sprintf(
		"provider_evt_integration_%d",
		time.Now().UnixNano(),
	)

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			`DELETE FROM payments WHERE payment_id = $1`,
			paymentID,
		)
	})

	createdAt := time.Now().UTC()

	payment := domain.Payment{
		ID:             paymentID,
		OrganizationID: integrationOrganizationID,
		MerchantID:     "integration_merchant",
		SessionID:      "integration_session",
		Provider:       "simulator",
		ProviderRef:    "integration_order",
		Expected: domain.Money{
			Currency: "KES",
			Minor:    1500,
		},
		CustomerDisplay: "Integration Test",
		Status:          domain.StatusRequested,
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}

	if err := repo.CreatePayment(ctx, payment); err != nil {
		t.Fatalf("create payment: %v", err)
	}

	stored, err := repo.GetPayment(ctx, paymentID)
	if err != nil {
		t.Fatalf("get payment: %v", err)
	}

	if stored.ID != paymentID {
		t.Fatalf("expected payment id %q, got %q", paymentID, stored.ID)
	}

	if stored.Status != domain.StatusRequested {
		t.Fatalf(
			"expected requested status, got %s",
			stored.Status,
		)
	}

	event := domain.PaymentEvent{
		EventID:         eventID,
		Provider:        "simulator",
		ProviderEventID: providerEventID,
		PaymentID:       paymentID,
		ProviderRef:     "integration_order",
		MerchantID:      "integration_merchant",
		Amount: domain.Money{
			Currency: "KES",
			Minor:    1500,
		},
		CustomerDisplay: "Integration Test",
		Kind:            "payment.pending",
		OccurredAt:      time.Now().UTC(),
	}

	match := verification.Match(stored, event)

	if !match.Matched {
		t.Fatalf("expected event to match: %s", match.Reason)
	}

	updated, err := repo.ApplyPaymentEvent(
		ctx,
		paymentID,
		event,
		domain.StatusPending,
		match,
	)
	if err != nil {
		t.Fatalf("apply payment event: %v", err)
	}

	if updated.Status != domain.StatusPending {
		t.Fatalf(
			"expected pending status, got %s",
			updated.Status,
		)
	}

	duplicate, err := repo.ApplyPaymentEvent(
		ctx,
		paymentID,
		event,
		domain.StatusPending,
		match,
	)
	if err != nil {
		t.Fatalf("duplicate event processing failed: %v", err)
	}

	if duplicate.Status != domain.StatusPending {
		t.Fatalf(
			"expected duplicate event to preserve pending status, got %s",
			duplicate.Status,
		)
	}

	var eventCount int

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM payment_events WHERE event_id = $1`,
		eventID,
	).Scan(&eventCount)
	if err != nil {
		t.Fatalf("count payment events: %v", err)
	}

	if eventCount != 1 {
		t.Fatalf(
			"expected exactly one stored event, got %d",
			eventCount,
		)
	}

	var verificationCount int

	err = pool.QueryRow(
		ctx,
		`
		SELECT COUNT(*)
		FROM payment_verifications
		WHERE event_id = (
			SELECT id
			FROM payment_events
			WHERE event_id = $1
		)
		`,
		eventID,
	).Scan(&verificationCount)
	if err != nil {
		t.Fatalf("count payment verifications: %v", err)
	}

	if verificationCount != 1 {
		t.Fatalf(
			"expected exactly one verification, got %d",
			verificationCount,
		)
	}
}
