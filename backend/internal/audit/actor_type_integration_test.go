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

const auditActorTestOrganizationID = "88888888-8888-4888-8888-888888888888"

func TestAuditActorTypeUser(t *testing.T) {
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

	ensureTestOrganization(t, ctx, pool)

	repository := postgres.NewAuditRepository(pool)
	service := audit.NewService(repository)

	resourceID := "actor-user-test"

	err = service.Record(ctx, audit.Event{
		OrganizationID: auditActorTestOrganizationID,
		ActorUserID:    "86b922e8-d3bd-479b-8a53-0302e51b0ba1",
		Action:         "test.user_actor",
		ResourceType:   "test",
		ResourceID:     resourceID,
		Metadata: map[string]any{
			"test": true,
		},
	})
	if err != nil {
		t.Fatalf("record user audit event: %v", err)
	}

	var (
		actorType string
		actorID   *string
	)

	err = pool.QueryRow(
		ctx,
		`
		SELECT
			actor_type,
			actor_user_id::text
		FROM audit_log
		WHERE organization_id = $1::uuid
		  AND action = $2
		  AND resource_id = $3
		ORDER BY created_at DESC
		LIMIT 1
		`,
		auditActorTestOrganizationID,
		"test.user_actor",
		resourceID,
	).Scan(&actorType, &actorID)

	if err != nil {
		t.Fatalf("read user audit event: %v", err)
	}

	if actorType != audit.ActorTypeUser {
		t.Fatalf(
			"expected actor type %q, got %q",
			audit.ActorTypeUser,
			actorType,
		)
	}

	if actorID == nil || *actorID == "" {
		t.Fatal("expected user actor id")
	}

	cleanupAuditActorTest(t, ctx, pool, resourceID)
}

func TestAuditActorTypeSystem(t *testing.T) {
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

	ensureTestOrganization(t, ctx, pool)

	repository := postgres.NewAuditRepository(pool)
	service := audit.NewService(repository)

	resourceID := "actor-system-test"

	err = service.Record(ctx, audit.Event{
		OrganizationID: auditActorTestOrganizationID,
		Action:         "test.system_actor",
		ResourceType:   "test",
		ResourceID:     resourceID,
		Metadata: map[string]any{
			"test": true,
		},
	})
	if err != nil {
		t.Fatalf("record system audit event: %v", err)
	}

	var (
		actorType string
		actorID   *string
	)

	err = pool.QueryRow(
		ctx,
		`
		SELECT
			actor_type,
			actor_user_id::text
		FROM audit_log
		WHERE organization_id = $1::uuid
		  AND action = $2
		  AND resource_id = $3
		ORDER BY created_at DESC
		LIMIT 1
		`,
		auditActorTestOrganizationID,
		"test.system_actor",
		resourceID,
	).Scan(&actorType, &actorID)

	if err != nil {
		t.Fatalf("read system audit event: %v", err)
	}

	if actorType != audit.ActorTypeSystem {
		t.Fatalf(
			"expected actor type %q, got %q",
			audit.ActorTypeSystem,
			actorType,
		)
	}

	if actorID != nil {
		t.Fatalf("expected NULL actor user id, got %q", *actorID)
	}

	cleanupAuditActorTest(t, ctx, pool, resourceID)
}

func ensureTestOrganization(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) {
	t.Helper()

	_, err := pool.Exec(
		ctx,
		`
		INSERT INTO organizations (
			id,
			name,
			status
		)
		VALUES (
			$1::uuid,
			$2,
			'active'
		)
		ON CONFLICT (id) DO NOTHING
		`,
		auditActorTestOrganizationID,
		"Werstics Verify Audit Actor Tests",
	)
	if err != nil {
		t.Fatalf("create test organization: %v", err)
	}
}

func cleanupAuditActorTest(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	resourceID string,
) {
	t.Helper()

	_, _ = pool.Exec(
		ctx,
		`
		DELETE FROM audit_log
		WHERE organization_id = $1::uuid
		  AND resource_id = $2
		`,
		auditActorTestOrganizationID,
		resourceID,
	)
}
