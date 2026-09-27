package ingestion

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
	"github.com/Namunyak2025/werstics-verify/backend/internal/payments"
	"github.com/Namunyak2025/werstics-verify/backend/internal/providers"
	"github.com/Namunyak2025/werstics-verify/backend/internal/verification"
)

type fakeFailureRepository struct {
	failure       providers.EventFailure
	recorded      *providers.EventFailure
	resolvedID    string
	resolvedOrgID string
}

func (f *fakeFailureRepository) Record(
	_ context.Context,
	failure providers.EventFailure,
) error {
	copy := failure
	f.recorded = &copy
	return nil
}

func (f *fakeFailureRepository) Get(
	_ context.Context,
	_ string,
	_ string,
) (providers.EventFailure, error) {
	return f.failure, nil
}

func (f *fakeFailureRepository) List(
	_ context.Context,
	_ string,
	_ string,
	_ int,
	_ int,
) ([]providers.EventFailure, int, error) {
	return []providers.EventFailure{f.failure}, 1, nil
}

func (f *fakeFailureRepository) Resolve(
	_ context.Context,
	id string,
	organizationID string,
) error {
	f.resolvedID = id
	f.resolvedOrgID = organizationID
	return nil
}

type fakePaymentRepository struct {
	payment     domain.Payment
	applyErr    error
	disposition domain.PaymentEventDisposition
}

func (f *fakePaymentRepository) CreatePayment(
	_ context.Context,
	_ domain.Payment,
) error {
	return nil
}

func (f *fakePaymentRepository) GetPayment(
	_ context.Context,
	_ string,
) (domain.Payment, error) {
	return f.payment, nil
}

func (f *fakePaymentRepository) ListPayments(
	_ context.Context,
	_ domain.PaymentFilter,
) ([]domain.Payment, int, error) {
	return []domain.Payment{f.payment}, 1, nil
}

func (f *fakePaymentRepository) FindPaymentByProviderRef(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) (domain.Payment, error) {
	return f.payment, nil
}

func (f *fakePaymentRepository) ApplyPaymentEvent(
	_ context.Context,
	_ string,
	_ domain.PaymentEvent,
	_ domain.PaymentStatus,
	_ verification.MatchResult,
) (domain.Payment, error) {
	if f.applyErr != nil {
		return f.payment, f.applyErr
	}

	f.payment.Status = domain.StatusConfirmed
	return f.payment, nil
}

func (f *fakePaymentRepository) ApplyPaymentEventDetailed(
	_ context.Context,
	_ string,
	_ domain.PaymentEvent,
	_ domain.PaymentStatus,
	_ verification.MatchResult,
) (domain.Payment, domain.PaymentEventDisposition, error) {
	if f.applyErr != nil {
		return f.payment, "", f.applyErr
	}

	f.payment.Status = domain.StatusConfirmed
	return f.payment, f.disposition, nil
}

func TestRetryFailureResolvesAfterSuccessfulProcessing(t *testing.T) {
	ctx := context.Background()
	orgID := "11111111-1111-4111-8111-111111111111"

	failures := &fakeFailureRepository{
		failure: providers.EventFailure{
			ID:              "failure-1",
			Provider:        "simulator",
			ProviderEventID: "provider-event-1",
			EventID:         "event-1",
			PaymentID:       "payment-1",
			ProviderRef:     "order-1",
			MerchantID:      "merchant-1",
			AmountCurrency:  "KES",
			AmountMinor:     1500,
			CustomerDisplay: "Customer",
			Kind:            "payment.confirmed",
			OccurredAt:      time.Now().UTC(),
			Status:          "retryable",
			Attempts:        1,
			LastError:       "temporary database error",
			FirstFailedAt:   time.Now().UTC(),
			LastFailedAt:    time.Now().UTC(),
		},
	}

	paymentsRepo := &fakePaymentRepository{
		payment: domain.Payment{
			ID:             "payment-1",
			OrganizationID: orgID,
			MerchantID:     "merchant-1",
			Provider:       "simulator",
			ProviderRef:    "order-1",
			Expected: domain.Money{
				Currency: "KES",
				Minor:    1500,
			},
			Status: domain.StatusPending,
		},
		disposition: domain.EventDispositionAccepted,
	}

	service := NewService(
		nil,
		payments.NewService(paymentsRepo),
		failures,
	)

	result, err := service.RetryFailure(
		ctx,
		"failure-1",
		orgID,
	)
	if err != nil {
		t.Fatalf("retry failure: %v", err)
	}

	if result.Payment.Status != domain.StatusConfirmed {
		t.Fatalf(
			"expected confirmed payment, got %s",
			result.Payment.Status,
		)
	}

	if failures.resolvedID != "failure-1" {
		t.Fatalf(
			"expected resolved failure-1, got %q",
			failures.resolvedID,
		)
	}

	if failures.resolvedOrgID != orgID {
		t.Fatalf(
			"expected organization %q, got %q",
			orgID,
			failures.resolvedOrgID,
		)
	}
}

func TestRetryFailureRecordsAnotherAttemptWhenProcessingFails(t *testing.T) {
	ctx := context.Background()
	orgID := "11111111-1111-4111-8111-111111111111"

	failures := &fakeFailureRepository{
		failure: providers.EventFailure{
			ID:              "failure-2",
			Provider:        "simulator",
			ProviderEventID: "provider-event-2",
			EventID:         "event-2",
			PaymentID:       "payment-2",
			ProviderRef:     "order-2",
			MerchantID:      "merchant-2",
			AmountCurrency:  "KES",
			AmountMinor:     1500,
			Kind:            "payment.confirmed",
			OccurredAt:      time.Now().UTC(),
			Status:          "retryable",
			Attempts:        2,
			LastError:       "temporary failure",
			FirstFailedAt:   time.Now().UTC(),
			LastFailedAt:    time.Now().UTC(),
		},
	}

	processingErr := errors.New("database unavailable")

	paymentsRepo := &fakePaymentRepository{
		payment: domain.Payment{
			ID:             "payment-2",
			OrganizationID: orgID,
			MerchantID:     "merchant-2",
			Provider:       "simulator",
			ProviderRef:    "order-2",
			Expected: domain.Money{
				Currency: "KES",
				Minor:    1500,
			},
			Status: domain.StatusPending,
		},
		applyErr: processingErr,
	}

	service := NewService(
		nil,
		payments.NewService(paymentsRepo),
		failures,
	)

	_, err := service.RetryFailure(
		ctx,
		"failure-2",
		orgID,
	)
	if err == nil {
		t.Fatal("expected retry failure")
	}

	if !errors.Is(err, ErrProcessingFailed) {
		t.Fatalf(
			"expected ErrProcessingFailed, got %v",
			err,
		)
	}

	if failures.recorded == nil {
		t.Fatal("expected retry failure to be recorded")
	}

	if failures.recorded.Attempts != 2 {
		t.Fatalf(
			"expected preserved attempt count 2, got %d",
			failures.recorded.Attempts,
		)
	}

	if failures.recorded.Status != "retryable" {
		t.Fatalf(
			"expected retryable status, got %q",
			failures.recorded.Status,
		)
	}
}
