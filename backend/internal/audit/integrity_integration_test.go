package audit_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	auditIntegrityOrgA = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	auditIntegrityOrgB = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
)

func TestAuditAppendOnlyIntegrity(t *testing.T) {
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

	ensureAuditIntegrityOrganizations(t, ctx, pool)

	resourceID := fmt.Sprintf(
		"audit-integrity-%d",
		time.Now().UnixNano(),
	)

	var auditID string

	err = pool.QueryRow(
		ctx,
		`
		INSERT INTO audit_log (
			id,
			organization_id,
			actor_type,
			action,
			resource_type,
			resource_id,
			metadata
		)
		VALUES (
			gen_random_uuid(),
			$1::uuid,
			'system',
			'audit.integrity_test',
			'test',
			$2,
			'{"test":true}'::jsonb
		)
		RETURNING id::text
		`,
		auditIntegrityOrgA,
		resourceID,
	).Scan(&auditID)
	if err != nil {
		t.Fatalf("insert audit record: %v", err)
	}

	if _, err := pool.Exec(
		ctx,
		`
		UPDATE audit_log
		SET action = 'audit.integrity_tampered'
		WHERE id = $1::uuid
		`,
		auditID,
	); err == nil {
		t.Fatal("expected audit UPDATE to be rejected")
	}

	if _, err := pool.Exec(
		ctx,
		`
		DELETE FROM audit_log
		WHERE id = $1::uuid
		`,
		auditID,
	); err == nil {
		t.Fatal("expected audit DELETE to be rejected")
	}
}

func TestAuditRejectsCrossOrganizationUserActor(t *testing.T) {
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

	ensureAuditIntegrityOrganizations(t, ctx, pool)

	userID := createAuditIntegrityUser(
		t,
		ctx,
		pool,
		auditIntegrityOrgA,
	)

	_, err = pool.Exec(
		ctx,
		`
		INSERT INTO audit_log (
			id,
			organization_id,
			actor_user_id,
			actor_type,
			action,
			resource_type,
			resource_id,
			metadata
		)
		VALUES (
			gen_random_uuid(),
			$1::uuid,
			$2::uuid,
			'user',
			'audit.cross_org_actor_test',
			'test',
			'cross-org',
			'{}'::jsonb
		)
		`,
		auditIntegrityOrgB,
		userID,
	)

	if err == nil {
		t.Fatal("expected cross-organization actor to be rejected")
	}
}

func TestAuditRejectsSystemActorWithUserID(t *testing.T) {
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

	ensureAuditIntegrityOrganizations(t, ctx, pool)

	userID := createAuditIntegrityUser(
		t,
		ctx,
		pool,
		auditIntegrityOrgA,
	)

	_, err = pool.Exec(
		ctx,
		`
		INSERT INTO audit_log (
			id,
			organization_id,
			actor_user_id,
			actor_type,
			action,
			resource_type,
			resource_id,
			metadata
		)
		VALUES (
			gen_random_uuid(),
			$1::uuid,
			$2::uuid,
			'system',
			'audit.invalid_system_actor_test',
			'test',
			'invalid-system',
			'{}'::jsonb
		)
		`,
		auditIntegrityOrgA,
		userID,
	)

	if err == nil {
		t.Fatal("expected system actor with actor_user_id to be rejected")
	}
}

func TestAuditRejectsUserActorWithoutUserID(t *testing.T) {
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

	ensureAuditIntegrityOrganizations(t, ctx, pool)

	_, err = pool.Exec(
		ctx,
		`
		INSERT INTO audit_log (
			id,
			organization_id,
			actor_type,
			action,
			resource_type,
			resource_id,
			metadata
		)
		VALUES (
			gen_random_uuid(),
			$1::uuid,
			'user',
			'audit.invalid_user_actor_test',
			'test',
			'missing-user',
			'{}'::jsonb
		)
		`,
		auditIntegrityOrgA,
	)

	if err == nil {
		t.Fatal("expected user actor without actor_user_id to be rejected")
	}
}

func ensureAuditIntegrityOrganizations(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) {
	t.Helper()

	for _, organization := range []struct {
		id   string
		name string
	}{
		{auditIntegrityOrgA, "Audit Integrity A"},
		{auditIntegrityOrgB, "Audit Integrity B"},
	} {
		_, err := pool.Exec(
			ctx,
			`
			INSERT INTO organizations (id, name, status)
			VALUES ($1::uuid, $2, 'active')
			ON CONFLICT (id) DO NOTHING
			`,
			organization.id,
			organization.name,
		)
		if err != nil {
			t.Fatalf("create organization: %v", err)
		}
	}
}

func createAuditIntegrityUser(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	organizationID string,
) string {
	t.Helper()

	var userID string

	err := pool.QueryRow(
		ctx,
		`
		INSERT INTO users (
			id,
			organization_id,
			email,
			password_hash,
			display_name,
			status
		)
		VALUES (
			gen_random_uuid(),
			$1::uuid,
			$2,
			'test-hash',
			'Audit Integrity User',
			'active'
		)
		RETURNING id::text
		`,
		organizationID,
		fmt.Sprintf(
			"audit-integrity-%d@example.invalid",
			time.Now().UnixNano(),
		),
	).Scan(&userID)
	if err != nil {
		t.Fatalf("create audit integrity user: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM users WHERE id = $1::uuid",
			userID,
		)
	})

	return userID
}

func TestAuditPreventsHardDeleteOfUserWithAuditHistory(t *testing.T) {
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

	ensureAuditIntegrityOrganizations(t, ctx, pool)

	userID := createAuditIntegrityUser(
		t,
		ctx,
		pool,
		auditIntegrityOrgA,
	)

	_, err = pool.Exec(
		ctx,
		`
		INSERT INTO audit_log (
			id,
			organization_id,
			actor_user_id,
			actor_type,
			action,
			resource_type,
			resource_id,
			metadata
		)
		VALUES (
			gen_random_uuid(),
			$1::uuid,
			$2::uuid,
			'user',
			'audit.user_delete_test',
			'test',
			'user-delete',
			'{}'::jsonb
		)
		`,
		auditIntegrityOrgA,
		userID,
	)
	if err != nil {
		t.Fatalf("insert audit history: %v", err)
	}

	_, err = pool.Exec(
		ctx,
		`
		DELETE FROM users
		WHERE id = $1::uuid
		`,
		userID,
	)
	if err == nil {
		t.Fatal("expected hard delete of audit-bearing user to be rejected")
	}
}
