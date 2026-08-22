package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Namunyak2025/werstics-verify/backend/internal/api"
)

type readinessChecker struct {
	err error
}

func (r readinessChecker) Ping(context.Context) error {
	return r.err
}

func newHealthServer(checker api.ReadinessChecker) *httptest.Server {
	server := api.NewServer(nil, nil, nil, nil)
	server.SetReadinessChecker(checker)
	return httptest.NewServer(server.Routes())
}

func TestHealthLive(t *testing.T) {
	server := newHealthServer(readinessChecker{})
	defer server.Close()

	resp, err := http.Get(server.URL + "/health/live")
	if err != nil {
		t.Fatalf("request health/live: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf(
			"expected health/live status 200, got %d",
			resp.StatusCode,
		)
	}
}

func TestHealthReadyWhenDatabaseIsReady(t *testing.T) {
	server := newHealthServer(readinessChecker{})
	defer server.Close()

	resp, err := http.Get(server.URL + "/health/ready")
	if err != nil {
		t.Fatalf("request health/ready: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf(
			"expected health/ready status 200, got %d",
			resp.StatusCode,
		)
	}
}

func TestHealthReadyWhenDatabaseIsUnavailable(t *testing.T) {
	server := newHealthServer(
		readinessChecker{
			err: errors.New("database unavailable"),
		},
	)
	defer server.Close()

	resp, err := http.Get(server.URL + "/health/ready")
	if err != nil {
		t.Fatalf("request health/ready: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf(
			"expected health/ready status 503, got %d",
			resp.StatusCode,
		)
	}
}

func TestHealthReadyWithoutChecker(t *testing.T) {
	server := api.NewServer(nil, nil, nil, nil)
	httpServer := httptest.NewServer(server.Routes())
	defer httpServer.Close()

	resp, err := http.Get(httpServer.URL + "/health/ready")
	if err != nil {
		t.Fatalf("request health/ready: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf(
			"expected health/ready status 503, got %d",
			resp.StatusCode,
		)
	}
}
