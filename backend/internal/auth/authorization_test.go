package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type testPermissionChecker struct {
	allowed         bool
	err             error
	permissionCalls int
}

func (f *testPermissionChecker) HasPermission(
	_ context.Context,
	_ string,
	_ string,
) (bool, error) {
	f.permissionCalls++
	return f.allowed, f.err
}

func (f *testPermissionChecker) ListPermissions(
	_ context.Context,
	_ string,
) ([]string, error) {
	return nil, nil
}

type testAuditRecorder struct {
	called         bool
	userID         string
	organizationID string
	permission     string
	resourceType   string
	resourceID     string
}

func (f *testAuditRecorder) RecordDenied(
	_ context.Context,
	user User,
	permission string,
	resourceType string,
	resourceID string,
) {
	f.called = true
	f.userID = user.ID
	f.organizationID = user.OrganizationID
	f.permission = permission
	f.resourceType = resourceType
	f.resourceID = resourceID
}

func testUser() User {
	return User{
		ID:             "86b922e8-d3bd-479b-8a53-0302e51b0ba1",
		OrganizationID: "11111111-1111-4111-8111-111111111111",
		Email:          "admin@werstics.local",
		DisplayName:    "Werstics Admin",
		Status:         "active",
	}
}

func requestWithUser(
	method string,
	path string,
	user User,
) *http.Request {
	req := httptest.NewRequest(method, path, nil)

	return req.WithContext(
		context.WithValue(
			req.Context(),
			userContextKey,
			user,
		),
	)
}

func TestRequirePermissionRejectsWithoutAuthentication(t *testing.T) {
	checker := &testPermissionChecker{allowed: true}
	recorder := &testAuditRecorder{}

	called := false

	handler := RequirePermission(
		checker,
		recorder,
		"payment:create",
		"payment",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/payments",
		nil,
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, req)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf(
			"expected 401, got %d",
			response.Code,
		)
	}

	if called {
		t.Fatal("protected handler must not execute")
	}

	if checker.permissionCalls != 0 {
		t.Fatalf(
			"permission checker should not run for unauthenticated request; got %d calls",
			checker.permissionCalls,
		)
	}

	if recorder.called {
		t.Fatal("unauthenticated request should not create permission-denied audit event")
	}

	if got := response.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Fatalf(
			"expected WWW-Authenticate Bearer, got %q",
			got,
		)
	}
}

func TestRequirePermissionRejectsAndAuditsDeniedPermission(t *testing.T) {
	checker := &testPermissionChecker{allowed: false}
	recorder := &testAuditRecorder{}

	called := false

	handler := RequirePermission(
		checker,
		recorder,
		"payment:create",
		"payment",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}),
	)

	req := requestWithUser(
		http.MethodPost,
		"/v1/payments",
		testUser(),
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, req)

	if response.Code != http.StatusForbidden {
		t.Fatalf(
			"expected 403, got %d",
			response.Code,
		)
	}

	if called {
		t.Fatal("protected handler must not execute")
	}

	if checker.permissionCalls != 1 {
		t.Fatalf(
			"expected one permission check, got %d",
			checker.permissionCalls,
		)
	}

	if !recorder.called {
		t.Fatal("expected denied action to be audited")
	}

	if recorder.userID != testUser().ID {
		t.Fatalf(
			"expected actor %q, got %q",
			testUser().ID,
			recorder.userID,
		)
	}

	if recorder.organizationID != testUser().OrganizationID {
		t.Fatalf(
			"expected organization %q, got %q",
			testUser().OrganizationID,
			recorder.organizationID,
		)
	}

	if recorder.permission != "payment:create" {
		t.Fatalf(
			"expected payment:create, got %q",
			recorder.permission,
		)
	}

	if recorder.resourceType != "payment" {
		t.Fatalf(
			"expected resource type payment, got %q",
			recorder.resourceType,
		)
	}

	if recorder.resourceID != "/v1/payments" {
		t.Fatalf(
			"expected resource id /v1/payments, got %q",
			recorder.resourceID,
		)
	}
}

func TestRequirePermissionAllowsAuthorizedRequest(t *testing.T) {
	checker := &testPermissionChecker{allowed: true}
	recorder := &testAuditRecorder{}

	called := false

	handler := RequirePermission(
		checker,
		recorder,
		"payment:create",
		"payment",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	req := requestWithUser(
		http.MethodPost,
		"/v1/payments",
		testUser(),
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, req)

	if response.Code != http.StatusNoContent {
		t.Fatalf(
			"expected 204, got %d",
			response.Code,
		)
	}

	if !called {
		t.Fatal("protected handler should execute")
	}

	if recorder.called {
		t.Fatal("authorized request must not create denial audit event")
	}

	if checker.permissionCalls != 1 {
		t.Fatalf(
			"expected one permission check, got %d",
			checker.permissionCalls,
		)
	}
}

func TestRequirePermissionReturnsInternalServerErrorOnCheckerFailure(t *testing.T) {
	checker := &testPermissionChecker{
		err: errors.New("database unavailable"),
	}
	recorder := &testAuditRecorder{}

	called := false

	handler := RequirePermission(
		checker,
		recorder,
		"payment:create",
		"payment",
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}),
	)

	req := requestWithUser(
		http.MethodPost,
		"/v1/payments",
		testUser(),
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, req)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf(
			"expected 500, got %d",
			response.Code,
		)
	}

	if called {
		t.Fatal("protected handler must not execute")
	}

	if recorder.called {
		t.Fatal("permission-check failure must not be recorded as permission denial")
	}
}

func TestCurrentPermissionsRejectsMissingUser(t *testing.T) {
	checker := &testPermissionChecker{}

	_, err := CurrentPermissions(
		context.Background(),
		checker,
	)

	if !errors.Is(err, ErrForbidden) {
		t.Fatalf(
			"expected ErrForbidden, got %v",
			err,
		)
	}
}
