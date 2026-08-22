package providers

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
)

type SimulatorAdapter struct {
	Secret string
}

func NewSimulatorAdapter(secret string) *SimulatorAdapter {
	return &SimulatorAdapter{
		Secret: secret,
	}
}

func (a *SimulatorAdapter) Name() string {
	return "simulator"
}

func (a *SimulatorAdapter) VerifySignature(
	_ context.Context,
	payload []byte,
	headers map[string]string,
) error {
	if a.Secret == "" {
		return fmt.Errorf("%w: simulator secret is not configured", ErrInvalidSignature)
	}

	provided := strings.TrimSpace(headers["X-Werstics-Signature"])
	if provided == "" {
		return ErrInvalidSignature
	}

	sum := sha256.Sum256(append(
		[]byte(a.Secret),
		payload...,
	))

	expected := hex.EncodeToString(sum[:])

	if subtle.ConstantTimeCompare(
		[]byte(provided),
		[]byte(expected),
	) != 1 {
		return ErrInvalidSignature
	}

	return nil
}

type simulatorPayload struct {
	EventID         string       `json:"event_id"`
	ProviderEventID string       `json:"provider_event_id"`
	PaymentID       string       `json:"payment_id"`
	ProviderRef     string       `json:"provider_ref"`
	MerchantID      string       `json:"merchant_id"`
	Amount          domain.Money `json:"amount"`
	CustomerDisplay string       `json:"customer_display"`
	Kind            string       `json:"kind"`
	OccurredAt      string       `json:"occurred_at"`
}

func (a *SimulatorAdapter) Normalize(
	_ context.Context,
	payload []byte,
) (domain.PaymentEvent, error) {
	var input simulatorPayload

	if err := json.Unmarshal(payload, &input); err != nil {
		return domain.PaymentEvent{}, fmt.Errorf(
			"%w: %v",
			ErrMalformedPayload,
			err,
		)
	}

	if input.EventID == "" ||
		input.ProviderEventID == "" ||
		input.PaymentID == "" ||
		input.MerchantID == "" ||
		input.Kind == "" ||
		input.Amount.Currency == "" ||
		input.Amount.Minor <= 0 ||
		input.OccurredAt == "" {
		return domain.PaymentEvent{}, ErrMalformedPayload
	}

	occurredAt, err := parseTimestamp(input.OccurredAt)
	if err != nil {
		return domain.PaymentEvent{}, err
	}

	return domain.PaymentEvent{
		EventID:         input.EventID,
		Provider:        a.Name(),
		ProviderEventID: input.ProviderEventID,
		PaymentID:       input.PaymentID,
		ProviderRef:     input.ProviderRef,
		MerchantID:      input.MerchantID,
		Amount:          input.Amount,
		CustomerDisplay: input.CustomerDisplay,
		Kind:            input.Kind,
		OccurredAt:      occurredAt,
	}, nil
}

func parseTimestamp(value string) (t time.Time, err error) {
	t, err = time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"%w: invalid occurred_at",
			ErrMalformedPayload,
		)
	}

	return t.UTC(), nil
}
