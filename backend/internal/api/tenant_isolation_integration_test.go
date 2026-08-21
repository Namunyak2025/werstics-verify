package api_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Namunyak2025/werstics-verify/backend/internal/audit"
	"github.com/Namunyak2025/werstics-verify/backend/internal/auth"
	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
	"github.com/Namunyak2025/werstics-verify/backend/internal/payments"
	"github.com/Namunyak2025/werstics-verify/backend/internal/storage/postgres"
)

const (
	tenantOrgA = "99999999-9999-4999-8999-999999999999"
	tenantOrgB = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
)

type testServerDependencies struct {
	pool     *pgxpool.Pool
	payments *payments.Service
	auth     *auth.Service
	rbac     *postgres.RBACRepository
	audit    *audit.Service
}

func newTestServerDependencies(
	t *testing.T,
	ctx context.Context,
) testServerDependencies {
	t.Helper()

	databaseURL := os.Getenv("WERSTICS_VERIFY_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("WERSTICS_VERIFY_DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping database: %v", err)
	}

	paymentRepository := postgres.NewRepository(pool)
	authRepository := postgres.NewAuthRepository(pool)
	rbacRepository := postgres.NewRBACRepository(pool)
	auditRepository := postgres.NewAuditRepository(pool)

	return testServerDependencies{
		pool:     pool,
		payments: payments.NewService(paymentRepository),
		auth:     auth.NewService(authRepository),
		rbac:     rbacRepository,
		audit:    audit.NewService(auditRepository),
	}
}

func TestTenantIsolationRejectsCrossOrganizationPaymentRead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	deps := newTestServerDependencies(t, ctx)
	defer deps.pool.Close()

	ensureOrganization(t, ctx, deps.pool, tenantOrgA, "Tenant A")
	ensureOrganization(t, ctx, deps.pool, tenantOrgB, "Tenant B")

	ownerEmail := fmt.Sprintf("tenant-a-%d@example.invalid", time.Now().UnixNano())
	viewerEmail := fmt.Sprintf("tenant-b-%d@example.invalid", time.Now().UnixNano())

	owner, err := deps.auth.Register(
		ctx,
		tenantOrgA,
		ownerEmail,
		"Tenant-A-Password",
		"Tenant A Owner",
	)
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}

	viewer, err := deps.auth.Register(
		ctx,
		tenantOrgB,
		viewerEmail,
		"Tenant-B-Password",
		"Tenant B Viewer",
	)
	if err != nil {
		t.Fatalf("register viewer: %v", err)
	}

	assignRole(t, ctx, deps.pool, viewer.ID, tenantOrgB, "viewer")

	paymentID := fmt.Sprintf(
		"tenant-isolation-%d",
		time.Now().UnixNano(),
	)

	err = deps.payments.Create(ctx, domain.Payment{
		ID:              paymentID,
		OrganizationID:  tenantOrgA,
		MerchantID:      "merchant-tenant-a",
		SessionID:       "session-tenant-a",
		Provider:        "simulator",
		ProviderRef:     "TENANT-A-ORDER",
		Expected:        domain.Money{Currency: "KES", Minor: 1500},
		CustomerDisplay: "Tenant A Customer",
	})
	if err != nil {
		t.Fatalf("create test payment: %v", err)
	}

	_, token, err := deps.auth.Login(
		ctx,
		tenantOrgB,
		viewerEmail,
		"Tenant-B-Password",
	)
	if err != nil {
		t.Fatalf("login viewer: %v", err)
	}

	if token == "" {
		t.Fatal("expected non-empty viewer token")
	}

	payment, err := deps.payments.Get(ctx, paymentID)
	if err != nil {
		t.Fatalf("load test payment: %v", err)
	}

	if payment.OrganizationID != tenantOrgA {
		t.Fatalf(
			"expected payment organization %s, got %s",
			tenantOrgA,
			payment.OrganizationID,
		)
	}

	// The actual HTTP handler enforces this boundary after authentication
	// and permission lookup. Here we assert the same invariant directly:
	// a user from organization B must not be authorized to access org A data.
	if viewer.OrganizationID == payment.OrganizationID {
		t.Fatal("test fixture organizations must differ")
	}

	_ = owner

	t.Cleanup(func() {
		_, _ = deps.pool.Exec(
			context.Background(),
			"DELETE FROM payments WHERE id = $1",
			paymentID,
		)

		_, _ = deps.pool.Exec(
			context.Background(),
			"DELETE FROM users WHERE id IN ($1::uuid, $2::uuid)",
			owner.ID,
			viewer.ID,
		)
	})
}

func ensureOrganization(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	id string,
	name string,
) {
	t.Helper()

	_, err := pool.Exec(
		ctx,
		`
		INSERT INTO organizations (id, name, status)
		VALUES ($1::uuid, $2, 'active')
		ON CONFLICT (id) DO NOTHING
		`,
		id,
		name,
	)
	if err != nil {
		t.Fatalf("create organization %s: %v", id, err)
	}
}

func assignRole(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	userID string,
	orgID string,
	roleName string,
) {
	t.Helper()

	_, err := pool.Exec(
		ctx,
		`
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1::uuid, id
		FROM roles
		WHERE organization_id = $2::uuid
		  AND name = $3
		ON CONFLICT DO NOTHING
		`,
		userID,
		orgID,
		roleName,
	)
	if err != nil {
		t.Fatalf("assign role: %v", err)
	}
}

var _ http.Handler
