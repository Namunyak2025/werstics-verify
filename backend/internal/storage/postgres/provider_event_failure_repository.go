package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Namunyak2025/werstics-verify/backend/internal/providers"
)

type ProviderEventFailureRepository struct {
	db *pgxpool.Pool
}

func NewProviderEventFailureRepository(
	db *pgxpool.Pool,
) *ProviderEventFailureRepository {
	return &ProviderEventFailureRepository{db: db}
}

func (r *ProviderEventFailureRepository) Record(
	ctx context.Context,
	failure providers.EventFailure,
) error {
	const query = `
		INSERT INTO provider_event_failures (
			id,
			provider,
			provider_event_id,
			event_id,
			payment_id,
			provider_ref,
			merchant_id,
			amount_currency,
			amount_minor,
			customer_display,
			kind,
			occurred_at,
			status,
			attempts,
			last_error,
			first_failed_at,
			last_failed_at
		)
		VALUES (
			gen_random_uuid(),
			$1,
			$2,
			$3,
			(
				SELECT id
				FROM payments
				WHERE payment_id = NULLIF($4, '')
			),
			NULLIF($5, ''),
			$6,
			$7,
			$8,
			NULLIF($9, ''),
			$10,
			$11,
			$12,
			$13,
			$14,
			$15,
			$16
		)
		ON CONFLICT (provider, provider_event_id)
		DO UPDATE SET
			event_id = EXCLUDED.event_id,
			payment_id = EXCLUDED.payment_id,
			provider_ref = EXCLUDED.provider_ref,
			merchant_id = EXCLUDED.merchant_id,
			amount_currency = EXCLUDED.amount_currency,
			amount_minor = EXCLUDED.amount_minor,
			customer_display = EXCLUDED.customer_display,
			kind = EXCLUDED.kind,
			occurred_at = EXCLUDED.occurred_at,
			status = EXCLUDED.status,
			attempts = provider_event_failures.attempts + 1,
			last_error = EXCLUDED.last_error,
			last_failed_at = EXCLUDED.last_failed_at,
			resolved_at = NULL
	`

	_, err := r.db.Exec(
		ctx,
		query,
		failure.Provider,
		failure.ProviderEventID,
		failure.EventID,
		failure.PaymentID,
		failure.ProviderRef,
		failure.MerchantID,
		failure.AmountCurrency,
		failure.AmountMinor,
		failure.CustomerDisplay,
		failure.Kind,
		failure.OccurredAt,
		failure.Status,
		failure.Attempts,
		failure.LastError,
		failure.FirstFailedAt,
		failure.LastFailedAt,
	)
	if err != nil {
		return fmt.Errorf(
			"record provider event failure: %w",
			err,
		)
	}

	return nil
}

func (r *ProviderEventFailureRepository) Get(
	ctx context.Context,
	id string,
	organizationID string,
) (providers.EventFailure, error) {
	const query = `
		SELECT
			f.id::text,
			f.provider,
			f.provider_event_id,
			f.event_id,
			COALESCE(p.payment_id, ''),
			COALESCE(f.provider_ref, ''),
			f.merchant_id,
			f.amount_currency,
			f.amount_minor,
			COALESCE(f.customer_display, ''),
			f.kind,
			f.occurred_at,
			f.status,
			f.attempts,
			f.last_error,
			f.first_failed_at,
			f.last_failed_at,
			f.resolved_at
		FROM provider_event_failures f
		LEFT JOIN payments p ON p.id = f.payment_id
		WHERE f.id = $1::uuid
		  AND p.organization_id = $2::uuid
	`

	var failure providers.EventFailure

	err := r.db.QueryRow(ctx, query, id, organizationID).Scan(
		&failure.ID,
		&failure.Provider,
		&failure.ProviderEventID,
		&failure.EventID,
		&failure.PaymentID,
		&failure.ProviderRef,
		&failure.MerchantID,
		&failure.AmountCurrency,
		&failure.AmountMinor,
		&failure.CustomerDisplay,
		&failure.Kind,
		&failure.OccurredAt,
		&failure.Status,
		&failure.Attempts,
		&failure.LastError,
		&failure.FirstFailedAt,
		&failure.LastFailedAt,
		&failure.ResolvedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return providers.EventFailure{}, ErrNotFound
	}
	if err != nil {
		return providers.EventFailure{}, fmt.Errorf(
			"get provider event failure: %w",
			err,
		)
	}

	return failure, nil
}

func (r *ProviderEventFailureRepository) List(
	ctx context.Context,
	organizationID string,
	status string,
	page int,
	pageSize int,
) ([]providers.EventFailure, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}

	const baseQuery = `
		FROM provider_event_failures f
		JOIN payments p ON p.id = f.payment_id
		WHERE p.organization_id = $1::uuid
		  AND ($2 = '' OR f.status = $2)
	`

	var total int

	if err := r.db.QueryRow(
		ctx,
		`SELECT COUNT(*) `+baseQuery,
		organizationID,
		status,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf(
			"count provider event failures: %w",
			err,
		)
	}

	offset := (page - 1) * pageSize

	rows, err := r.db.Query(
		ctx,
		`
		SELECT
			f.id::text,
			f.provider,
			f.provider_event_id,
			f.event_id,
			COALESCE(
				(
					SELECT p.payment_id
					FROM payments p
					WHERE p.id = f.payment_id
				),
				''
			),
			COALESCE(f.provider_ref, ''),
			f.merchant_id,
			f.amount_currency,
			f.amount_minor,
			COALESCE(f.customer_display, ''),
			f.kind,
			f.occurred_at,
			f.status,
			f.attempts,
			f.last_error,
			f.first_failed_at,
			f.last_failed_at,
			f.resolved_at
		`+baseQuery+`
		ORDER BY f.last_failed_at DESC, f.id DESC
		LIMIT $3
		OFFSET $4
		`,
		organizationID,
		status,
		pageSize,
		offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf(
			"query provider event failures: %w",
			err,
		)
	}
	defer rows.Close()

	failures := make([]providers.EventFailure, 0, pageSize)

	for rows.Next() {
		var failure providers.EventFailure

		if err := rows.Scan(
			&failure.ID,
			&failure.Provider,
			&failure.ProviderEventID,
			&failure.EventID,
			&failure.PaymentID,
			&failure.ProviderRef,
			&failure.MerchantID,
			&failure.AmountCurrency,
			&failure.AmountMinor,
			&failure.CustomerDisplay,
			&failure.Kind,
			&failure.OccurredAt,
			&failure.Status,
			&failure.Attempts,
			&failure.LastError,
			&failure.FirstFailedAt,
			&failure.LastFailedAt,
			&failure.ResolvedAt,
		); err != nil {
			return nil, 0, fmt.Errorf(
				"scan provider event failure: %w",
				err,
			)
		}

		failures = append(failures, failure)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf(
			"iterate provider event failures: %w",
			err,
		)
	}

	return failures, total, nil
}

func (r *ProviderEventFailureRepository) Resolve(
	ctx context.Context,
	id string,
	organizationID string,
) error {
	const query = `
		UPDATE provider_event_failures
		SET
			status = 'resolved',
			resolved_at = NOW(),
			last_failed_at = NOW()
		WHERE id = $1::uuid
		  AND payment_id IN (
			SELECT id
			FROM payments
			WHERE organization_id = $2::uuid
		  )
	`

	result, err := r.db.Exec(ctx, query, id, organizationID)
	if err != nil {
		return fmt.Errorf(
			"resolve provider event failure: %w",
			err,
		)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
