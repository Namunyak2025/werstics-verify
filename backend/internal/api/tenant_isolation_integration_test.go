package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Namunyak2025/werstics-verify/backend/internal/api"

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

	adminEmail := fmt.Sprintf(
		"tenant-b-admin-%d@example.invalid",
		time.Now().UnixNano(),
	)

	admin, err := deps.auth.Register(
		ctx,
		tenantOrgB,
		adminEmail,
		"Tenant-B-Password",
		"Tenant B Admin",
	)
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}

	assignRole(t, ctx, deps.pool, admin.ID, tenantOrgB, "admin")

	paymentID := fmt.Sprintf(
		"tenant-isolation-read-%d",
		time.Now().UnixNano(),
	)

	if err := deps.payments.Create(
		ctx,
		domain.Payment{
			ID:              paymentID,
			OrganizationID:  tenantOrgA,
			MerchantID:      "merchant-tenant-a",
			SessionID:       "session-tenant-a",
			Provider:        "simulator",
			ProviderRef:     "TENANT-A-ORDER",
			Expected:        domain.Money{Currency: "KES", Minor: 1500},
			CustomerDisplay: "Tenant A Customer",
		},
	); err != nil {
		t.Fatalf("create test payment: %v", err)
	}

	_, token, err := deps.auth.Login(
		ctx,
		tenantOrgB,
		adminEmail,
		"Tenant-B-Password",
	)
	if err != nil {
		t.Fatalf("login admin: %v", err)
	}

	server := api.NewServer(
		deps.payments,
		deps.auth,
		deps.rbac,
		deps.audit,
	)

	httpServer := httptest.NewServer(server.Routes())
	defer httpServer.Close()

	req, err := http.NewRequest(
		http.MethodGet,
		httpServer.URL+"/v1/payments/"+paymentID,
		nil,
	)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf(
			"expected cross-tenant read to return 403, got %d",
			resp.StatusCode,
		)
	}

	assertAuditAction(
		t,
		ctx,
		deps.pool,
		tenantOrgB,
		"payment.read_denied",
		paymentID,
	)

	cleanupTenantFixtures(t, ctx, deps.pool, admin.ID, paymentID)
}

func TestTenantIsolationRejectsCrossOrganizationPaymentVerification(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	deps := newTestServerDependencies(t, ctx)
	defer deps.pool.Close()

	ensureOrganization(t, ctx, deps.pool, tenantOrgA, "Tenant A")
	ensureOrganization(t, ctx, deps.pool, tenantOrgB, "Tenant B")

	adminEmail := fmt.Sprintf(
		"tenant-b-verify-admin-%d@example.invalid",
		time.Now().UnixNano(),
	)

	admin, err := deps.auth.Register(
		ctx,
		tenantOrgB,
		adminEmail,
		"Tenant-B-Password",
		"Tenant B Verify Admin",
	)
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}

	assignRole(t, ctx, deps.pool, admin.ID, tenantOrgB, "admin")

	paymentID := fmt.Sprintf(
		"tenant-isolation-verify-%d",
		time.Now().UnixNano(),
	)

	if err := deps.payments.Create(
		ctx,
		domain.Payment{
			ID:             paymentID,
			OrganizationID: tenantOrgA,
			MerchantID:     "merchant-tenant-a",
			SessionID:      "session-tenant-a-verify",
			Provider:       "simulator",
			ProviderRef:    "TENANT-A-VERIFY",
			Expected:       domain.Money{Currency: "KES", Minor: 1500},
		},
	); err != nil {
		t.Fatalf("create test payment: %v", err)
	}

	_, token, err := deps.auth.Login(
		ctx,
		tenantOrgB,
		adminEmail,
		"Tenant-B-Password",
	)
	if err != nil {
		t.Fatalf("login admin: %v", err)
	}

	server := api.NewServer(
		deps.payments,
		deps.auth,
		deps.rbac,
		deps.audit,
	)

	httpServer := httptest.NewServer(server.Routes())
	defer httpServer.Close()

	body := map[string]any{
		"event_id":          "cross-tenant-event",
		"provider":          "simulator",
		"provider_event_id": "cross-tenant-provider-event",
		"provider_ref":      "TENANT-A-VERIFY",
		"merchant_id":       "merchant-tenant-a",
		"amount": map[string]any{
			"currency": "KES",
			"minor":    1500,
		},
		"kind": "payment.pending",
	}

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode verification body: %v", err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		httpServer.URL+"/v1/payments/"+paymentID+"/events",
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf(
			"expected cross-tenant verification to return 403, got %d",
			resp.StatusCode,
		)
	}

	assertAuditAction(
		t,
		ctx,
		deps.pool,
		tenantOrgB,
		"payment.verification_denied",
		paymentID,
	)

	cleanupTenantFixtures(t, ctx, deps.pool, admin.ID, paymentID)
}

func TestTenantIsolationRejectsCrossOrganizationPaymentCreate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	deps := newTestServerDependencies(t, ctx)
	defer deps.pool.Close()

	ensureOrganization(t, ctx, deps.pool, tenantOrgA, "Tenant A")
	ensureOrganization(t, ctx, deps.pool, tenantOrgB, "Tenant B")

	adminEmail := fmt.Sprintf(
		"tenant-b-create-admin-%d@example.invalid",
		time.Now().UnixNano(),
	)

	admin, err := deps.auth.Register(
		ctx,
		tenantOrgB,
		adminEmail,
		"Tenant-B-Password",
		"Tenant B Create Admin",
	)
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}

	assignRole(t, ctx, deps.pool, admin.ID, tenantOrgB, "admin")

	_, token, err := deps.auth.Login(
		ctx,
		tenantOrgB,
		adminEmail,
		"Tenant-B-Password",
	)
	if err != nil {
		t.Fatalf("login admin: %v", err)
	}

	server := api.NewServer(
		deps.payments,
		deps.auth,
		deps.rbac,
		deps.audit,
	)

	httpServer := httptest.NewServer(server.Routes())
	defer httpServer.Close()

	paymentID := fmt.Sprintf(
		"tenant-isolation-create-%d",
		time.Now().UnixNano(),
	)

	body := map[string]any{
		"id":              paymentID,
		"organization_id": tenantOrgA,
		"merchant_id":     "merchant-cross-tenant",
		"session_id":      "session-cross-tenant",
		"provider":        "simulator",
		"provider_ref":    "CROSS-TENANT-CREATE",
		"expected": map[string]any{
			"currency": "KES",
			"minor":    1500,
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode payment body: %v", err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		httpServer.URL+"/v1/payments",
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf(
			"expected cross-tenant create to return 403, got %d",
			resp.StatusCode,
		)
	}

	assertAuditAction(
		t,
		ctx,
		deps.pool,
		tenantOrgB,
		"payment.create_denied",
		paymentID,
	)

	cleanupTenantFixtures(t, ctx, deps.pool, admin.ID, paymentID)
}

func assertAuditAction(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	organizationID string,
	action string,
	resourceID string,
) {
	t.Helper()

	var count int

	err := pool.QueryRow(
		ctx,
		`
		SELECT COUNT(*)
		FROM audit_log
		WHERE organization_id = $1::uuid
		  AND action = $2
		  AND resource_id = $3
		`,
		organizationID,
		action,
		resourceID,
	).Scan(&count)

	if err != nil {
		t.Fatalf("query audit denial: %v", err)
	}

	if count < 1 {
		t.Fatalf(
			"expected audit action %q for resource %q",
			action,
			resourceID,
		)
	}
}

func cleanupTenantFixtures(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	userID string,
	paymentID string,
) {
	t.Helper()

	_, _ = pool.Exec(
		ctx,
		"DELETE FROM payments WHERE payment_id = $1",
		paymentID,
	)

	_, _ = pool.Exec(
		ctx,
		"DELETE FROM users WHERE id = $1::uuid",
		userID,
	)
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
