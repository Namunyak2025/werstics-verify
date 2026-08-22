package audit_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Namunyak2025/werstics-verify/backend/internal/audit"
	"github.com/Namunyak2025/werstics-verify/backend/internal/storage/postgres"
)

func TestAuditListIsOrganizationScoped(t *testing.T) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
	)
	defer cancel()

	databaseURL := os.Getenv("WERSTICS_VERIFY_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("WERSTICS_VERIFY_DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}

	repository := postgres.NewAuditRepository(pool)

	records, total, err := repository.List(
		ctx,
		audit.Filter{
			OrganizationID: "11111111-1111-4111-8111-111111111111",
			Page:           1,
			PageSize:       25,
		},
	)
	if err != nil {
		t.Fatalf("list audit records: %v", err)
	}

	if total == 0 {
		t.Fatal("expected audit records for test organization")
	}

	if len(records) == 0 {
		t.Fatal("expected returned audit records")
	}

	for _, record := range records {
		if record.OrganizationID != "11111111-1111-4111-8111-111111111111" {
			t.Fatalf(
				"cross-organization audit record returned: %s",
				record.OrganizationID,
			)
		}

		if record.ActorType != audit.ActorTypeUser &&
			record.ActorType != audit.ActorTypeSystem {
			t.Fatalf(
				"invalid actor type %q",
				record.ActorType,
			)
		}
	}
}
