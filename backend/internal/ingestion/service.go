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
	ErrProcessingFailed = errors.New("provider event processing failed")
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
				"%w: %v",
				ErrProcessingFailed,
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
	result, err := s.IngestDetailed(
		ctx,
		provider,
		payload,
		headers,
	)

	return result.Payment, err
}
