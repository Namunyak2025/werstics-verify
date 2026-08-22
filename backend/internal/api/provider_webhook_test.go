package api_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Namunyak2025/werstics-verify/backend/internal/api"
	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
)

type fakeProviderIngestion struct {
	payment domain.Payment
	err     error

	provider string
	payload  []byte
	headers  map[string]string
}

func (f *fakeProviderIngestion) Ingest(
	_ context.Context,
	provider string,
	payload []byte,
	headers map[string]string,
) (domain.Payment, error) {
	f.provider = provider
	f.payload = append([]byte(nil), payload...)

	f.headers = make(map[string]string, len(headers))
	for key, value := range headers {
		f.headers[key] = value
	}

	return f.payment, f.err
}

func newWebhookServer(
	ingestion api.ProviderIngestion,
) *httptest.Server {
	server := api.NewServer(nil, nil, nil, nil)
	server.SetProviderIngestion(ingestion)

	return httptest.NewServer(server.Routes())
}

func TestProviderWebhookAcceptsPostWithoutUserAuthentication(
	t *testing.T,
) {
	fake := &fakeProviderIngestion{
		payment: domain.Payment{
			ID:             "pay-webhook-001",
			OrganizationID: "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
			Provider:       "simulator",
		},
	}

	server := newWebhookServer(fake)
	defer server.Close()

	payload := []byte(`{"event_id":"evt-1"}`)

	req, err := http.NewRequest(
		http.MethodPost,
		server.URL+"/v1/providers/simulator/webhook",
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	req.Header.Set("X-Werstics-Signature", "test-signature")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if fake.provider != "simulator" {
		t.Fatalf(
			"expected provider simulator, got %q",
			fake.provider,
		)
	}

	if !bytes.Equal(fake.payload, payload) {
		t.Fatal("webhook payload was not forwarded unchanged")
	}

	if fake.headers["X-Werstics-Signature"] != "test-signature" {
		t.Fatal("webhook signature header was not forwarded")
	}
}

func TestProviderWebhookRejectsWrongMethod(t *testing.T) {
	server := newWebhookServer(nil)
	defer server.Close()

	req, err := http.NewRequest(
		http.MethodGet,
		server.URL+"/v1/providers/simulator/webhook",
		nil,
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

func TestProviderWebhookRejectsUnknownProvider(t *testing.T) {
	fake := &fakeProviderIngestion{
		err: errors.New("unsupported payment provider: unknown"),
	}

	server := newWebhookServer(fake)
	defer server.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		server.URL+"/v1/providers/unknown/webhook",
		bytes.NewBufferString(`{}`),
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound &&
		resp.StatusCode != http.StatusBadRequest {
		t.Fatalf(
			"expected provider rejection status, got %d",
			resp.StatusCode,
		)
	}
}

func TestProviderWebhookRejectsInvalidSignature(t *testing.T) {
	fake := &fakeProviderIngestion{
		err: errors.New(
			"verify provider signature: invalid provider signature",
		),
	}

	server := newWebhookServer(fake)
	defer server.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		server.URL+"/v1/providers/simulator/webhook",
		bytes.NewBufferString(`{}`),
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf(
			"expected 401, got %d",
			resp.StatusCode,
		)
	}
}

func TestProviderWebhookRejectsMalformedPayload(t *testing.T) {
	fake := &fakeProviderIngestion{
		err: errors.New(
			"normalize provider event: malformed provider payload",
		),
	}

	server := newWebhookServer(fake)
	defer server.Close()

	req, err := http.NewRequest(
		http.MethodPost,
		server.URL+"/v1/providers/simulator/webhook",
		bytes.NewBufferString(`{}`),
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
