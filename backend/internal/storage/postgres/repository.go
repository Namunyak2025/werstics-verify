package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
	"github.com/Namunyak2025/werstics-verify/backend/internal/verification"
)

var (
	ErrNotFound       = errors.New("record not found")
	ErrDuplicateEvent = errors.New("duplicate payment event")
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreatePayment(
	ctx context.Context,
	payment domain.Payment,
) error {
	const query = `
		INSERT INTO payments (
			id,
			payment_id,
			organization_id,
			merchant_id,
			session_id,
			provider,
			provider_ref,
			expected_currency,
			expected_minor,
			customer_display,
			status,
			created_at,
			updated_at
		)
		VALUES (
			gen_random_uuid(),
			$1,
			$2::uuid,
			$3,
			$4,
			$5,
			NULLIF($6, ''),
			$7,
			$8,
			NULLIF($9, ''),
			$10,
			$11,
			$12
		)
	`

	_, err := r.db.Exec(
		ctx,
		query,
		payment.ID,
		payment.OrganizationID,
		payment.MerchantID,
		payment.SessionID,
		payment.Provider,
		payment.ProviderRef,
		payment.Expected.Currency,
		payment.Expected.Minor,
		payment.CustomerDisplay,
		payment.Status,
		payment.CreatedAt,
		payment.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("insert payment: %w", err)
	}

	return nil
}

func (r *Repository) GetPayment(
	ctx context.Context,
	paymentID string,
) (domain.Payment, error) {
	const query = `
		SELECT
			payment_id,
			organization_id::text,
			merchant_id,
			session_id,
			provider,
			COALESCE(provider_ref, ''),
			expected_currency,
			expected_minor,
			received_currency,
			received_minor,
			COALESCE(customer_display, ''),
			status,
			created_at,
			updated_at
		FROM payments
		WHERE payment_id = $1
	`

	var (
		payment          domain.Payment
		expectedCurrency string
		expectedMinor    int64
		receivedCurrency *string
		receivedMinor    *int64
		status           string
	)

	err := r.db.QueryRow(ctx, query, paymentID).Scan(
		&payment.ID,
		&payment.OrganizationID,
		&payment.MerchantID,
		&payment.SessionID,
		&payment.Provider,
		&payment.ProviderRef,
		&expectedCurrency,
		&expectedMinor,
		&receivedCurrency,
		&receivedMinor,
		&payment.CustomerDisplay,
		&status,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Payment{}, ErrNotFound
	}

	if err != nil {
		return domain.Payment{}, fmt.Errorf("query payment: %w", err)
	}

	payment.Expected = domain.Money{
		Currency: expectedCurrency,
		Minor:    expectedMinor,
	}

	payment.Status = domain.PaymentStatus(status)

	if receivedCurrency != nil && receivedMinor != nil {
		payment.Received = &domain.Money{
			Currency: *receivedCurrency,
			Minor:    *receivedMinor,
		}
	}

	return payment, nil
}

func (r *Repository) ApplyPaymentEvent(
	ctx context.Context,
	paymentID string,
	event domain.PaymentEvent,
	target domain.PaymentStatus,
	match verification.MatchResult,
) (domain.Payment, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Payment{}, fmt.Errorf("begin payment event transaction: %w", err)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const paymentQuery = `
		SELECT
			id,
			payment_id,
			organization_id::text,
			merchant_id,
			session_id,
			provider,
			COALESCE(provider_ref, ''),
			expected_currency,
			expected_minor,
			received_currency,
			received_minor,
			COALESCE(customer_display, ''),
			status,
			created_at,
			updated_at
		FROM payments
		WHERE payment_id = $1
		FOR UPDATE
	`

	var (
		payment          domain.Payment
		internalID       string
		expectedCurrency string
		expectedMinor    int64
		receivedCurrency *string
		receivedMinor    *int64
		status           string
	)

	err = tx.QueryRow(ctx, paymentQuery, paymentID).Scan(
		&internalID,
		&payment.ID,
		&payment.OrganizationID,
		&payment.MerchantID,
		&payment.SessionID,
		&payment.Provider,
		&payment.ProviderRef,
		&expectedCurrency,
		&expectedMinor,
		&receivedCurrency,
		&receivedMinor,
		&payment.CustomerDisplay,
		&status,
		&payment.CreatedAt,
		&payment.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Payment{}, ErrNotFound
	}

	if err != nil {
		return domain.Payment{}, fmt.Errorf("lock payment: %w", err)
	}

	payment.Expected = domain.Money{
		Currency: expectedCurrency,
		Minor:    expectedMinor,
	}

	payment.Status = domain.PaymentStatus(status)

	if receivedCurrency != nil && receivedMinor != nil {
		payment.Received = &domain.Money{
			Currency: *receivedCurrency,
			Minor:    *receivedMinor,
		}
	}

	const insertEvent = `
		INSERT INTO payment_events (
			id,
			payment_id,
			event_id,
			provider,
			provider_event_id,
			provider_ref,
			merchant_id,
			amount_currency,
			amount_minor,
			customer_display,
			kind,
			occurred_at,
			processing_status
		)
		VALUES (
			gen_random_uuid(),
			$1,
			$2,
			$3,
			$4,
			NULLIF($5, ''),
			$6,
			$7,
			$8,
			NULLIF($9, ''),
			$10,
			$11,
			$12
		)
		ON CONFLICT DO NOTHING
		RETURNING id
	`

	var eventInternalID string

	err = tx.QueryRow(
		ctx,
		insertEvent,
		internalID,
		event.EventID,
		event.Provider,
		event.ProviderEventID,
		event.ProviderRef,
		event.MerchantID,
		event.Amount.Currency,
		event.Amount.Minor,
		event.CustomerDisplay,
		event.Kind,
		event.OccurredAt,
		processingStatus(match),
	).Scan(&eventInternalID)

	if errors.Is(err, pgx.ErrNoRows) {
		return r.handleDuplicateEvent(ctx, tx, payment, event)
	}

	if err != nil {
		return domain.Payment{}, fmt.Errorf("insert payment event: %w", err)
	}

	const insertVerification = `
		INSERT INTO payment_verifications (
			id,
			payment_id,
			event_id,
			matched,
			amount_matched,
			merchant_matched,
			reason
		)
		VALUES (
			gen_random_uuid(),
			$1,
			$2,
			$3,
			$4,
			$5,
			$6
		)
	`

	_, err = tx.Exec(
		ctx,
		insertVerification,
		internalID,
		eventInternalID,
		match.Matched,
		match.AmountMatched,
		match.MerchantMatch,
		match.Reason,
	)

	if err != nil {
		return domain.Payment{}, fmt.Errorf("insert payment verification: %w", err)
	}

	if match.Matched {
		if err := domain.ValidateTransition(payment.Status, target); err != nil {
			return domain.Payment{}, err
		}

		const updatePayment = `
			UPDATE payments
			SET
				received_currency = $1,
				received_minor = $2,
				provider_ref = NULLIF($3, ''),
				customer_display = NULLIF($4, ''),
				status = $5,
				updated_at = NOW()
			WHERE id = $6
		`

		_, err = tx.Exec(
			ctx,
			updatePayment,
			event.Amount.Currency,
			event.Amount.Minor,
			event.ProviderRef,
			event.CustomerDisplay,
			target,
			internalID,
		)

		if err != nil {
			return domain.Payment{}, fmt.Errorf("update payment state: %w", err)
		}

		payment.Received = &domain.Money{
			Currency: event.Amount.Currency,
			Minor:    event.Amount.Minor,
		}
		payment.ProviderRef = event.ProviderRef
		payment.CustomerDisplay = event.CustomerDisplay
		payment.Status = target
	}

	payment.UpdatedAt = payment.UpdatedAt.UTC()

	if err := tx.Commit(ctx); err != nil {
		return domain.Payment{}, fmt.Errorf("commit payment event: %w", err)
	}

	return payment, nil
}

func (r *Repository) handleDuplicateEvent(
	ctx context.Context,
	tx pgx.Tx,
	payment domain.Payment,
	event domain.PaymentEvent,
) (domain.Payment, error) {
	const eventQuery = `
		SELECT id
		FROM payment_events
		WHERE event_id = $1
		   OR (
				provider = $2
				AND provider_event_id = $3
		   )
		LIMIT 1
	`

	var eventInternalID string

	err := tx.QueryRow(
		ctx,
		eventQuery,
		event.EventID,
		event.Provider,
		event.ProviderEventID,
	).Scan(&eventInternalID)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Payment{}, ErrDuplicateEvent
	}

	if err != nil {
		return domain.Payment{}, fmt.Errorf("find duplicate event: %w", err)
	}

	const verificationQuery = `
		SELECT
			matched,
			amount_matched,
			merchant_matched,
			reason
		FROM payment_verifications
		WHERE event_id = $1
	`

	var result verification.MatchResult

	err = tx.QueryRow(
		ctx,
		verificationQuery,
		eventInternalID,
	).Scan(
		&result.Matched,
		&result.AmountMatched,
		&result.MerchantMatch,
		&result.Reason,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Payment{}, ErrDuplicateEvent
	}

	if err != nil {
		return domain.Payment{}, fmt.Errorf("find duplicate verification: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Payment{}, fmt.Errorf("commit duplicate event transaction: %w", err)
	}

	return payment, nil
}

func processingStatus(match verification.MatchResult) string {
	if match.Matched {
		return "processed"
	}

	return "rejected"
}
