package payments

import (
	"context"
	"fmt"
	"time"

	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
	"github.com/Namunyak2025/werstics-verify/backend/internal/verification"
)

type Repository interface {
	CreatePayment(ctx context.Context, payment domain.Payment) error
	GetPayment(ctx context.Context, paymentID string) (domain.Payment, error)
	FindPaymentByProviderRef(
		ctx context.Context,
		provider string,
		providerRef string,
		merchantID string,
	) (domain.Payment, error)
	ListPayments(ctx context.Context, filter domain.PaymentFilter) ([]domain.Payment, int, error)
	ApplyPaymentEvent(
		ctx context.Context,
		paymentID string,
		event domain.PaymentEvent,
		target domain.PaymentStatus,
		match verification.MatchResult,
	) (domain.Payment, error)
	ApplyPaymentEventDetailed(
		ctx context.Context,
		paymentID string,
		event domain.PaymentEvent,
		target domain.PaymentStatus,
		match verification.MatchResult,
	) (domain.Payment, domain.PaymentEventDisposition, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) FindByProviderRef(
	ctx context.Context,
	provider string,
	providerRef string,
	merchantID string,
) (domain.Payment, error) {
	if provider == "" {
		return domain.Payment{}, fmt.Errorf("provider is required")
	}

	if providerRef == "" {
		return domain.Payment{}, fmt.Errorf("provider reference is required")
	}

	if merchantID == "" {
		return domain.Payment{}, fmt.Errorf("merchant id is required")
	}

	payment, err := s.repo.FindPaymentByProviderRef(
		ctx,
		provider,
		providerRef,
		merchantID,
	)
	if err != nil {
		return domain.Payment{}, fmt.Errorf(
			"find payment by provider reference: %w",
			err,
		)
	}

	return payment, nil
}

func (s *Service) List(
	ctx context.Context,
	filter domain.PaymentFilter,
) ([]domain.Payment, int, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}

	if filter.PageSize < 1 {
		filter.PageSize = 25
	}

	if filter.PageSize > 100 {
		filter.PageSize = 100
	}

	if filter.OrganizationID == "" {
		return nil, 0, fmt.Errorf("organization id is required")
	}

	payments, total, err := s.repo.ListPayments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("list payments: %w", err)
	}

	return payments, total, nil
}

func (s *Service) Create(ctx context.Context, payment domain.Payment) error {
	if err := payment.Validate(); err != nil {
		return err
	}

	now := time.Now().UTC()

	payment.Status = domain.StatusRequested
	payment.CreatedAt = now
	payment.UpdatedAt = now

	if err := s.repo.CreatePayment(ctx, payment); err != nil {
		return fmt.Errorf("create payment: %w", err)
	}

	return nil
}

func (s *Service) Get(ctx context.Context, paymentID string) (domain.Payment, error) {
	payment, err := s.repo.GetPayment(ctx, paymentID)
	if err != nil {
		return domain.Payment{}, fmt.Errorf("get payment: %w", err)
	}

	return payment, nil
}

func (s *Service) ApplyEventDetailed(
	ctx context.Context,
	event domain.PaymentEvent,
) (domain.Payment, domain.PaymentEventDisposition, verification.MatchResult, error) {
	payment, err := s.repo.GetPayment(ctx, event.PaymentID)
	if err != nil {
		return domain.Payment{}, "", verification.MatchResult{}, fmt.Errorf(
			"get payment for event: %w",
			err,
		)
	}

	target, err := targetStatus(event.Kind)
	if err != nil {
		return payment, "", verification.MatchResult{}, err
	}

	match := verification.Match(payment, event)

	if !match.Matched {
		_, disposition, err := s.repo.ApplyPaymentEventDetailed(
			ctx,
			payment.ID,
			event,
			payment.Status,
			match,
		)
		if err != nil {
			return payment, disposition, match, fmt.Errorf(
				"persist unmatched payment event: %w",
				err,
			)
		}

		return payment, disposition, match, nil
	}

	updated, disposition, err := s.repo.ApplyPaymentEventDetailed(
		ctx,
		payment.ID,
		event,
		target,
		match,
	)
	if err != nil {
		return payment, disposition, match, fmt.Errorf(
			"apply payment event: %w",
			err,
		)
	}

	return updated, disposition, match, nil
}

func (s *Service) ApplyEvent(
	ctx context.Context,
	event domain.PaymentEvent,
) (domain.Payment, verification.MatchResult, error) {
	payment, err := s.repo.GetPayment(ctx, event.PaymentID)
	if err != nil {
		return domain.Payment{}, verification.MatchResult{}, fmt.Errorf(
			"get payment for event: %w",
			err,
		)
	}

	target, err := targetStatus(event.Kind)
	if err != nil {
		return payment, verification.MatchResult{}, err
	}

	match := verification.Match(payment, event)

	if !match.Matched {
		_, err := s.repo.ApplyPaymentEvent(
			ctx,
			payment.ID,
			event,
			payment.Status,
			match,
		)
		if err != nil {
			return payment, match, fmt.Errorf(
				"persist unmatched payment event: %w",
				err,
			)
		}

		return payment, match, nil
	}

	if err := domain.ValidateTransition(payment.Status, target); err != nil {
		return payment, match, err
	}

	updated, err := s.repo.ApplyPaymentEvent(
		ctx,
		payment.ID,
		event,
		target,
		match,
	)
	if err != nil {
		return payment, match, fmt.Errorf(
			"apply payment event: %w",
			err,
		)
	}

	return updated, match, nil
}

func targetStatus(kind string) (domain.PaymentStatus, error) {
	switch kind {
	case "payment.pending":
		return domain.StatusPending, nil
	case "payment.confirmed":
		return domain.StatusConfirmed, nil
	case "payment.settled":
		return domain.StatusSettled, nil
	case "payment.failed":
		return domain.StatusFailed, nil
	case "payment.expired":
		return domain.StatusExpired, nil
	case "payment.reversed":
		return domain.StatusReversed, nil
	case "payment.refunded":
		return domain.StatusRefunded, nil
	case "payment.cancelled":
		return domain.StatusCancelled, nil
	default:
		return "", fmt.Errorf("unsupported event kind %q", kind)
	}
}
