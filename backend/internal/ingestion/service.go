package ingestion

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	failures providers.FailureRepository
}

func NewService(
	registry *providers.Registry,
	paymentService *payments.Service,
	failureRecorder providers.FailureRepository,
) *Service {
	return &Service{
		registry: registry,
		payments: paymentService,
		failures: failureRecorder,
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
		if event.ProviderRef == "" || event.MerchantID == "" {
			return Result{}, fmt.Errorf(
				"%w: payment correlation requires provider_ref and merchant_id",
				providers.ErrMalformedPayload,
			)
		}

		payment, err := s.payments.FindByProviderRef(
			ctx,
			event.Provider,
			event.ProviderRef,
			event.MerchantID,
		)
		if err != nil {
			return Result{}, fmt.Errorf(
				"correlate provider event to payment: %w",
				err,
			)
		}

		event.PaymentID = payment.ID
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
		now := time.Now().UTC()

		failure := providers.EventFailure{
			Provider:        event.Provider,
			ProviderEventID: event.ProviderEventID,
			EventID:         event.EventID,
			PaymentID:       event.PaymentID,
			ProviderRef:     event.ProviderRef,
			MerchantID:      event.MerchantID,
			AmountCurrency:  event.Amount.Currency,
			AmountMinor:     event.Amount.Minor,
			CustomerDisplay: event.CustomerDisplay,
			Kind:            event.Kind,
			OccurredAt:      event.OccurredAt,
			Status:          "retryable",
			Attempts:        1,
			LastError:       err.Error(),
			FirstFailedAt:   now,
			LastFailedAt:    now,
		}

		if s.failures != nil {
			if recordErr := s.failures.Record(ctx, failure); recordErr != nil {
				return Result{
						Payment:     updated,
						Disposition: disposition,
						Event:       event,
					}, fmt.Errorf(
						"%w: %v; record failure: %v",
						ErrProcessingFailed,
						err,
						recordErr,
					)
			}
		}

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

func (s *Service) RetryFailure(
	ctx context.Context,
	id string,
	organizationID string,
) (Result, error) {
	if s.failures == nil {
		return Result{}, fmt.Errorf("failure repository is not configured")
	}

	failure, err := s.failures.Get(ctx, id, organizationID)
	if err != nil {
		return Result{}, err
	}

	if failure.Status == "resolved" {
		return Result{
			Payment: domain.Payment{
				ID: failure.PaymentID,
			},
			Event: domain.PaymentEvent{
				EventID:         failure.EventID,
				Provider:        failure.Provider,
				ProviderEventID: failure.ProviderEventID,
				PaymentID:       failure.PaymentID,
				ProviderRef:     failure.ProviderRef,
				MerchantID:      failure.MerchantID,
				Amount: domain.Money{
					Currency: failure.AmountCurrency,
					Minor:    failure.AmountMinor,
				},
				CustomerDisplay: failure.CustomerDisplay,
				Kind:            failure.Kind,
				OccurredAt:      failure.OccurredAt,
			},
		}, nil
	}

	event := domain.PaymentEvent{
		EventID:         failure.EventID,
		Provider:        failure.Provider,
		ProviderEventID: failure.ProviderEventID,
		PaymentID:       failure.PaymentID,
		ProviderRef:     failure.ProviderRef,
		MerchantID:      failure.MerchantID,
		Amount: domain.Money{
			Currency: failure.AmountCurrency,
			Minor:    failure.AmountMinor,
		},
		CustomerDisplay: failure.CustomerDisplay,
		Kind:            failure.Kind,
		OccurredAt:      failure.OccurredAt,
	}

	updated, disposition, _, err := s.payments.ApplyEventDetailed(
		ctx,
		event,
	)
	if err != nil {
		now := time.Now().UTC()

		next := providers.EventFailure{
			ID:              failure.ID,
			Provider:        failure.Provider,
			ProviderEventID: failure.ProviderEventID,
			EventID:         failure.EventID,
			PaymentID:       failure.PaymentID,
			ProviderRef:     failure.ProviderRef,
			MerchantID:      failure.MerchantID,
			AmountCurrency:  failure.AmountCurrency,
			AmountMinor:     failure.AmountMinor,
			CustomerDisplay: failure.CustomerDisplay,
			Kind:            failure.Kind,
			OccurredAt:      failure.OccurredAt,
			Status:          "retryable",
			Attempts:        failure.Attempts,
			LastError:       err.Error(),
			FirstFailedAt:   failure.FirstFailedAt,
			LastFailedAt:    now,
		}

		if recordErr := s.failures.Record(ctx, next); recordErr != nil {
			return Result{
					Payment:     updated,
					Disposition: disposition,
					Event:       event,
				}, fmt.Errorf(
					"%w: %v; record retry failure: %v",
					ErrProcessingFailed,
					err,
					recordErr,
				)
		}

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

	if err := s.failures.Resolve(
		ctx,
		failure.ID,
		organizationID,
	); err != nil {
		return Result{
			Payment:     updated,
			Disposition: disposition,
			Event:       event,
		}, fmt.Errorf("resolve provider event failure: %w", err)
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
