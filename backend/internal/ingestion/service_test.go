package ingestion_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/Namunyak2025/werstics-verify/backend/internal/ingestion"
	"github.com/Namunyak2025/werstics-verify/backend/internal/providers"
	"strings"
	"testing"
)

func TestServiceRejectsMissingProvider(t *testing.T) {
	service := ingestion.NewService(nil, nil)

	_, err := service.Ingest(
		context.Background(),
		"",
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("expected missing provider to fail")
	}
}

func TestServiceRejectsUnknownProvider(t *testing.T) {
	registry := providers.NewRegistry()
	service := ingestion.NewService(registry, nil)

	_, err := service.Ingest(
		context.Background(),
		"unknown",
		[]byte(`{}`),
		nil,
	)
	if err == nil {
		t.Fatal("expected unknown provider to fail")
	}
}

func TestServiceRejectsInvalidSignatureBeforeNormalization(t *testing.T) {
	registry := providers.NewRegistry(
		providers.NewSimulatorAdapter("secret"),
	)

	service := ingestion.NewService(registry, nil)

	_, err := service.Ingest(
		context.Background(),
		"simulator",
		[]byte(`{"event_id":"evt-1"}`),
		map[string]string{
			"X-Werstics-Signature": "invalid",
		},
	)
	if err == nil {
		t.Fatal("expected invalid signature to fail")
	}

	if !strings.Contains(err.Error(), "verify provider signature") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func simulatorSignature(secret string, payload []byte) string {
	sum := sha256.Sum256(append([]byte(secret), payload...))
	return hex.EncodeToString(sum[:])
}
