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
	ApplyPaymentEvent(
		ctx context.Context,
		paymentID string,
		event domain.PaymentEvent,
		target domain.PaymentStatus,
		match verification.MatchResult,
	) (domain.Payment, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
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

	target, err := targetStatus(event.Kind)
	if err != nil {
		return payment, match, err
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
