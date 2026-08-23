package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Namunyak2025/werstics-verify/backend/internal/providers"
)

func TestProviderEventFailureRecordIsIdempotent(t *testing.T) {
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

	repo := NewProviderEventFailureRepository(pool)

	suffix := time.Now().UnixNano()
	providerEventID := fmt.Sprintf("failure-provider-%d", suffix)
	eventID := fmt.Sprintf("failure-event-%d", suffix)

	failure := providers.EventFailure{
		Provider:        "simulator",
		ProviderEventID: providerEventID,
		EventID:         eventID,
		MerchantID:      "merchant_failure_test",
		AmountCurrency:  "KES",
		AmountMinor:     1500,
		CustomerDisplay: "Failure Test",
		Kind:            "payment.confirmed",
		OccurredAt:      time.Now().UTC(),
		Status:          "retryable",
		Attempts:        1,
		LastError:       "database timeout",
		FirstFailedAt:   time.Now().UTC(),
		LastFailedAt:    time.Now().UTC(),
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			`DELETE FROM provider_event_failures
			 WHERE provider = $1 AND provider_event_id = $2`,
			failure.Provider,
			failure.ProviderEventID,
		)
	})

	if err := repo.Record(ctx, failure); err != nil {
		t.Fatalf("record first failure: %v", err)
	}

	if err := repo.Record(ctx, failure); err != nil {
		t.Fatalf("record repeated failure: %v", err)
	}

	var (
		count    int
		attempts int
		status   string
	)

	err = pool.QueryRow(
		ctx,
		`
		SELECT COUNT(*), MAX(attempts), MAX(status)
		FROM provider_event_failures
		WHERE provider = $1
		  AND provider_event_id = $2
		`,
		failure.Provider,
		failure.ProviderEventID,
	).Scan(&count, &attempts, &status)
	if err != nil {
		t.Fatalf("query failure record: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected one failure record, got %d", count)
	}

	if attempts != 2 {
		t.Fatalf("expected two attempts, got %d", attempts)
	}

	if status != "retryable" {
		t.Fatalf("expected retryable status, got %q", status)
	}
}
