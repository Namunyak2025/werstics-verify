package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Namunyak2025/werstics-verify/backend/internal/auth"
	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
	"github.com/Namunyak2025/werstics-verify/backend/internal/ingestion"
	"github.com/Namunyak2025/werstics-verify/backend/internal/providers"
)

const recoveryTestOrganization = "11111111-1111-4111-8111-111111111111"

type recoveryRBACFake struct {
	allowed bool
}

func (f *recoveryRBACFake) HasPermission(
	context.Context,
	string,
	string,
) (bool, error) {
	return f.allowed, nil
}

func (f *recoveryRBACFake) ListPermissions(
	context.Context,
	string,
) ([]string, error) {
	return nil, nil
}

type recoveryFailureFake struct {
	items      []providers.EventFailure
	resolveID  string
	resolveOrg string
	listErr    error
	resolveErr error
}

func (f *recoveryFailureFake) Record(
	_ context.Context,
	failure providers.EventFailure,
) error {
	f.items = append(f.items, failure)
	return nil
}

func (f *recoveryFailureFake) Get(
	_ context.Context,
	_ string,
	_ string,
) (providers.EventFailure, error) {
	if len(f.items) == 0 {
		return providers.EventFailure{}, errors.New("record not found")
	}

	return f.items[0], nil
}

func (f *recoveryFailureFake) List(
	_ context.Context,
	_ string,
	_ string,
	_ int,
	_ int,
) ([]providers.EventFailure, int, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}

	return f.items, len(f.items), nil
}

func (f *recoveryFailureFake) Resolve(
	_ context.Context,
	id string,
	organizationID string,
) error {
	if f.resolveErr != nil {
		return f.resolveErr
	}

	f.resolveID = id
	f.resolveOrg = organizationID
	return nil
}

type recoveryIngestionFake struct {
	result  ingestion.Result
	err     error
	lastID  string
	lastOrg string
}

func (f *recoveryIngestionFake) IngestDetailed(
	context.Context,
	string,
	[]byte,
	map[string]string,
) (ingestion.Result, error) {
	return f.result, f.err
}

func (f *recoveryIngestionFake) RetryFailure(
	_ context.Context,
	id string,
	organizationID string,
) (ingestion.Result, error) {
	f.lastID = id
	f.lastOrg = organizationID
	return f.result, f.err
}

type recoveryAuthRepository struct {
	user    auth.UserRepositoryRecord
	session auth.Session
}

func (r *recoveryAuthRepository) CreateUser(
	context.Context,
	auth.UserRepositoryRecord,
) (auth.UserRepositoryRecord, error) {
	return auth.UserRepositoryRecord{}, errors.New("not implemented")
}

func (r *recoveryAuthRepository) FindUserByEmail(
	context.Context,
	string,
	string,
) (auth.UserRepositoryRecord, error) {
	return r.user, nil
}

func (r *recoveryAuthRepository) FindUserByID(
	context.Context,
	string,
) (auth.UserRepositoryRecord, error) {
	return r.user, nil
}

func (r *recoveryAuthRepository) UpdateLastLogin(
	context.Context,
	string,
) error {
	return nil
}

func (r *recoveryAuthRepository) CreateSession(
	context.Context,
	string,
	string,
	time.Time,
) (string, error) {
	return r.session.ID, nil
}

func (r *recoveryAuthRepository) FindActiveSession(
	context.Context,
	string,
) (auth.Session, error) {
	return r.session, nil
}

func (r *recoveryAuthRepository) TouchSession(
	context.Context,
	string,
) error {
	return nil
}

func (r *recoveryAuthRepository) RevokeSession(
	context.Context,
	string,
) error {
	return nil
}

func recoveryHTTPServer(
	allowed bool,
	failures providers.FailureRepository,
	ingester ProviderIngestion,
) (*httptest.Server, string) {
	user := auth.UserRepositoryRecord{
		ID:             "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		OrganizationID: recoveryTestOrganization,
		Email:          "admin@werstics.local",
		DisplayName:    "Werstics Admin",
		Status:         "active",
	}

	session := auth.Session{
		ID:         "session-recovery-test",
		UserID:     user.ID,
		TokenHash:  auth.HashSessionToken("recovery-test-token"),
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
		CreatedAt:  time.Now().UTC(),
		LastSeenAt: time.Now().UTC(),
	}

	authService := auth.NewService(
		&recoveryAuthRepository{
			user:    user,
			session: session,
		},
	)

	server := &Server{
		auth:      authService,
		rbac:      &recoveryRBACFake{allowed: allowed},
		failures:  failures,
		ingestion: ingester,
	}

	handler := auth.Middleware(authService)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/v1/provider-failures":
				server.providerFailuresHandler(w, r)
			default:
				server.providerFailureActionHandler(w, r)
			}
		}),
	)

	return httptest.NewServer(handler), "Bearer recovery-test-token"
}

func TestProviderFailuresListRequiresAuthentication(t *testing.T) {
	server := &Server{
		rbac:     &recoveryRBACFake{allowed: true},
		failures: &recoveryFailureFake{},
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/v1/provider-failures",
		nil,
	)

	rec := httptest.NewRecorder()

	server.providerFailuresHandler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestProviderFailuresListPermissionDenied(t *testing.T) {
	failures := &recoveryFailureFake{}

	httpServer, token := recoveryHTTPServer(
		false,
		failures,
		&recoveryIngestionFake{},
	)
	defer httpServer.Close()

	req, err := http.NewRequest(
		http.MethodGet,
		httpServer.URL+"/v1/provider-failures",
		nil,
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	req.Header.Set("Authorization", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestProviderFailuresListReturnsFailures(t *testing.T) {
	failures := &recoveryFailureFake{
		items: []providers.EventFailure{
			{
				ID:              "failure-1",
				Provider:        "simulator",
				ProviderEventID: "provider-event-1",
				EventID:         "event-1",
				PaymentID:       "payment-1",
				Status:          "retryable",
				Attempts:        1,
			},
		},
	}

	httpServer, token := recoveryHTTPServer(
		true,
		failures,
		&recoveryIngestionFake{},
	)
	defer httpServer.Close()

	req, err := http.NewRequest(
		http.MethodGet,
		httpServer.URL+"/v1/provider-failures?status=retryable",
		nil,
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	req.Header.Set("Authorization", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestProviderFailureRetryUsesAuthenticatedOrganization(t *testing.T) {
	failures := &recoveryFailureFake{
		items: []providers.EventFailure{
			{
				ID:        "failure-1",
				PaymentID: "payment-1",
				Status:    "retryable",
			},
		},
	}

	ingester := &recoveryIngestionFake{
		result: ingestion.Result{
			Payment: domain.Payment{
				ID: "payment-1",
			},
			Disposition: domain.EventDispositionAccepted,
		},
	}

	httpServer, token := recoveryHTTPServer(
		true,
		failures,
		ingester,
	)
	defer httpServer.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		httpServer.URL+"/v1/provider-failures/failure-1/retry",
		nil,
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	req.Header.Set("Authorization", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if ingester.lastID != "failure-1" {
		t.Fatalf(
			"expected failure-1, got %q",
			ingester.lastID,
		)
	}

	if ingester.lastOrg != recoveryTestOrganization {
		t.Fatalf(
			"expected organization %q, got %q",
			recoveryTestOrganization,
			ingester.lastOrg,
		)
	}
}

func TestProviderFailureRetryMissingFailureReturns404(t *testing.T) {
	ingester := &recoveryIngestionFake{
		err: errors.New("record not found"),
	}

	httpServer, token := recoveryHTTPServer(
		true,
		&recoveryFailureFake{},
		ingester,
	)
	defer httpServer.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		httpServer.URL+"/v1/provider-failures/missing/retry",
		nil,
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	req.Header.Set("Authorization", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestProviderFailureResolveUsesAuthenticatedOrganization(t *testing.T) {
	failures := &recoveryFailureFake{
		items: []providers.EventFailure{
			{
				ID:     "failure-1",
				Status: "retryable",
			},
		},
	}

	httpServer, token := recoveryHTTPServer(
		true,
		failures,
		&recoveryIngestionFake{},
	)
	defer httpServer.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		httpServer.URL+"/v1/provider-failures/failure-1/resolve",
		nil,
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	req.Header.Set("Authorization", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if failures.resolveID != "failure-1" {
		t.Fatalf(
			"expected failure-1, got %q",
			failures.resolveID,
		)
	}

	if failures.resolveOrg != recoveryTestOrganization {
		t.Fatalf(
			"expected organization %q, got %q",
			recoveryTestOrganization,
			failures.resolveOrg,
		)
	}
}
