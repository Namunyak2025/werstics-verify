package providers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestSimulatorVerifySignature(t *testing.T) {
	adapter := NewSimulatorAdapter("secret")
	payload := []byte(`{"event_id":"evt-1"}`)

	sum := sha256.Sum256(append([]byte("secret"), payload...))
	signature := hex.EncodeToString(sum[:])

	if err := adapter.VerifySignature(
		context.Background(),
		payload,
		map[string]string{
			"X-Werstics-Signature": signature,
		},
	); err != nil {
		t.Fatalf("expected valid signature, got %v", err)
	}
}

func TestSimulatorRejectsInvalidSignature(t *testing.T) {
	adapter := NewSimulatorAdapter("secret")

	err := adapter.VerifySignature(
		context.Background(),
		[]byte(`{"event_id":"evt-1"}`),
		map[string]string{
			"X-Werstics-Signature": "invalid",
		},
	)
	if err == nil {
		t.Fatal("expected invalid signature to be rejected")
	}
}

func TestSimulatorNormalizeForcesProviderIdentity(t *testing.T) {
	adapter := NewSimulatorAdapter("secret")

	payload := []byte(`{
		"event_id":"evt-1",
		"provider":"attacker",
		"provider_event_id":"provider-evt-1",
		"payment_id":"pay-1",
		"provider_ref":"ORDER-1",
		"merchant_id":"merchant-1",
		"amount":{"currency":"KES","minor":1500},
		"kind":"payment.confirmed",
		"occurred_at":"2026-08-23T01:00:00Z"
	}`)

	event, err := adapter.Normalize(
		context.Background(),
		payload,
	)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}

	if event.Provider != "simulator" {
		t.Fatalf(
			"expected normalized provider simulator, got %q",
			event.Provider,
		)
	}
}

func TestSimulatorRejectsMalformedPayload(t *testing.T) {
	adapter := NewSimulatorAdapter("secret")

	_, err := adapter.Normalize(
		context.Background(),
		[]byte(`{"event_id":"evt-1"}`),
	)
	if err == nil {
		t.Fatal("expected malformed payload to be rejected")
	}
}
