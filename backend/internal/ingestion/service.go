package ingestion

import (
	"context"
	"errors"
	"fmt"

	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
	"github.com/Namunyak2025/werstics-verify/backend/internal/payments"
	"github.com/Namunyak2025/werstics-verify/backend/internal/providers"
)

var (
	ErrProviderRequired = errors.New("provider is required")
)

type Service struct {
	registry *providers.Registry
	payments *payments.Service
}

func NewService(
	registry *providers.Registry,
	paymentService *payments.Service,
) *Service {
	return &Service{
		registry: registry,
		payments: paymentService,
	}
}

type Result struct {
	Payment     domain.Payment
	Disposition domain.PaymentEventDisposition
	Event       domain.PaymentEvent
}

func (s *Service) IngestDetailed(
	ctx context.Context,
	provider string,
	payload []byte,
	headers map[string]string,
) (Result, error) {
	if provider == "" {
		return Result{}, ErrProviderRequired
	}

	if s.registry == nil {
		return Result{}, fmt.Errorf(
			"provider registry is not configured",
		)
	}

	adapter, err := s.registry.Get(provider)
	if err != nil {
		return Result{}, err
	}

	if err := adapter.VerifySignature(
		ctx,
		payload,
		headers,
	); err != nil {
		return Result{}, fmt.Errorf(
			"verify provider signature: %w",
			err,
		)
	}

	event, err := adapter.Normalize(ctx, payload)
	if err != nil {
		return Result{}, fmt.Errorf(
			"normalize provider event: %w",
			err,
		)
	}

	if event.Provider != adapter.Name() {
		return Result{}, fmt.Errorf(
			"normalized event provider %q does not match adapter %q",
			event.Provider,
			adapter.Name(),
		)
	}

	if s.payments == nil {
		return Result{}, fmt.Errorf(
			"payment service is not configured",
		)
	}

	if event.PaymentID == "" {
		return Result{}, fmt.Errorf(
			"%w: payment_id",
			providers.ErrMalformedPayload,
		)
	}

	updated, disposition, _, err := s.payments.ApplyEventDetailed(
		ctx,
		event,
	)
	if err != nil {
		return Result{
				Payment:     updated,
				Disposition: disposition,
				Event:       event,
			}, fmt.Errorf(
				"apply normalized provider event: %w",
				err,
			)
	}

	return Result{
		Payment:     updated,
		Disposition: disposition,
		Event:       event,
	}, nil
}

func (s *Service) Ingest(
	ctx context.Context,
	provider string,
	payload []byte,
	headers map[string]string,
) (domain.Payment, error) {
	if provider == "" {
		return domain.Payment{}, ErrProviderRequired
	}

	if s.registry == nil {
		return domain.Payment{}, fmt.Errorf(
			"provider registry is not configured",
		)
	}

	adapter, err := s.registry.Get(provider)
	if err != nil {
		return domain.Payment{}, err
	}

	if err := adapter.VerifySignature(
		ctx,
		payload,
		headers,
	); err != nil {
		return domain.Payment{}, fmt.Errorf(
			"verify provider signature: %w",
			err,
		)
	}

	event, err := adapter.Normalize(ctx, payload)
	if err != nil {
		return domain.Payment{}, fmt.Errorf(
			"normalize provider event: %w",
			err,
		)
	}

	if event.Provider != adapter.Name() {
		return domain.Payment{}, fmt.Errorf(
			"normalized event provider %q does not match adapter %q",
			event.Provider,
			adapter.Name(),
		)
	}

	if s.payments == nil {
		return domain.Payment{}, fmt.Errorf(
			"payment service is not configured",
		)
	}

	if event.PaymentID == "" {
		return domain.Payment{}, fmt.Errorf(
			"%w: payment_id",
			providers.ErrMalformedPayload,
		)
	}

	updated, _, err := s.payments.ApplyEvent(
		ctx,
		event,
	)
	if err != nil {
		return domain.Payment{}, fmt.Errorf(
			"apply normalized provider event: %w",
			err,
		)
	}

	return updated, nil
}
