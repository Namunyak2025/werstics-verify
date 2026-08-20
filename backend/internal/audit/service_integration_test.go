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

const auditTestOrganizationID = "77777777-7777-4777-8777-777777777777"

func TestAuditRecordPersistence(t *testing.T) {
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
		auditTestOrganizationID,
		"Werstics Verify Audit Tests",
	)
	if err != nil {
		t.Fatalf("create test organization: %v", err)
	}

	repository := postgres.NewAuditRepository(pool)
	service := audit.NewService(repository)

	event := audit.Event{
		OrganizationID: auditTestOrganizationID,
		Action:         "payment.verification_completed",
		ResourceType:   "payment",
		ResourceID:     "pay_audit_test",
		Metadata: map[string]any{
			"event_id":         "evt_audit_test",
			"matched":          true,
			"amount_matched":   true,
			"merchant_matched": true,
			"status":           "confirmed",
		},
	}

	if err := service.Record(ctx, event); err != nil {
		t.Fatalf("record audit event: %v", err)
	}

	var (
		action       string
		resourceID   string
		metadata     []byte
		organization string
	)

	err = pool.QueryRow(
		ctx,
		`
		SELECT
			organization_id::text,
			action,
			resource_id,
			metadata::text
		FROM audit_log
		WHERE organization_id = $1::uuid
		  AND action = $2
		  AND resource_id = $3
		ORDER BY created_at DESC
		LIMIT 1
		`,
		auditTestOrganizationID,
		"payment.verification_completed",
		"pay_audit_test",
	).Scan(
		&organization,
		&action,
		&resourceID,
		&metadata,
	)

	if err != nil {
		t.Fatalf("read audit event: %v", err)
	}

	if organization != auditTestOrganizationID {
		t.Fatalf(
			"expected organization %s, got %s",
			auditTestOrganizationID,
			organization,
		)
	}

	if action != event.Action {
		t.Fatalf(
			"expected action %s, got %s",
			event.Action,
			action,
		)
	}

	if resourceID != event.ResourceID {
		t.Fatalf(
			"expected resource id %s, got %s",
			event.ResourceID,
			resourceID,
		)
	}

	if len(metadata) == 0 {
		t.Fatal("expected audit metadata")
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			`
			DELETE FROM audit_log
			WHERE organization_id = $1::uuid
			  AND resource_id = $2
			`,
			auditTestOrganizationID,
			"pay_audit_test",
		)
	})
}

func TestAuditMetadataDoesNotContainCredentials(t *testing.T) {
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

	repository := postgres.NewAuditRepository(pool)
	service := audit.NewService(repository)

	event := audit.Event{
		OrganizationID: auditTestOrganizationID,
		Action:         "auth.login",
		ResourceType:   "user",
		ResourceID:     "user_audit_test",
		Metadata: map[string]any{
			"email": "audit-test@example.invalid",
		},
	}

	if err := service.Record(ctx, event); err != nil {
		t.Fatalf("record audit event: %v", err)
	}

	var metadata string

	err = pool.QueryRow(
		ctx,
		`
		SELECT metadata::text
		FROM audit_log
		WHERE organization_id = $1::uuid
		  AND action = $2
		  AND resource_id = $3
		ORDER BY created_at DESC
		LIMIT 1
		`,
		auditTestOrganizationID,
		"auth.login",
		"user_audit_test",
	).Scan(&metadata)

	if err != nil {
		t.Fatalf("read audit metadata: %v", err)
	}

	for _, forbidden := range []string{
		"password",
		"password_hash",
		"token",
		"token_hash",
		"authorization",
	} {
		if containsInsensitive(metadata, forbidden) {
			t.Fatalf(
				"audit metadata contains forbidden credential field %q: %s",
				forbidden,
				metadata,
			)
		}
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			`
			DELETE FROM audit_log
			WHERE organization_id = $1::uuid
			  AND resource_id = $2
			`,
			auditTestOrganizationID,
			"user_audit_test",
		)
	})
}

func containsInsensitive(value, needle string) bool {
	// Kept deliberately simple for test-only credential-field detection.
	for i := 0; i+len(needle) <= len(value); i++ {
		match := true

		for j := range needle {
			a := value[i+j]
			b := needle[j]

			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}

			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}

			if a != b {
				match = false
				break
			}
		}

		if match {
			return true
		}
	}

	return false
}
