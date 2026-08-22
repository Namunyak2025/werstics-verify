package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Namunyak2025/werstics-verify/backend/internal/audit"
)

type AuditRepository struct {
	db *pgxpool.Pool
}

func NewAuditRepository(db *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) Record(
	ctx context.Context,
	event audit.Event,
) error {
	const query = `
		INSERT INTO audit_log (
			id,
			organization_id,
			actor_user_id,
			actor_type,
			action,
			resource_type,
			resource_id,
			metadata,
			created_at
		)
		VALUES (
			gen_random_uuid(),
			NULLIF($1, '')::uuid,
			NULLIF($2, '')::uuid,
			$3,
			$4,
			$5,
			NULLIF($6, ''),
			$7::jsonb,
			$8
		)
	`

	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}

	_, err = r.db.Exec(
		ctx,
		query,
		event.OrganizationID,
		event.ActorUserID,
		event.ActorType,
		event.Action,
		event.ResourceType,
		event.ResourceID,
		string(metadata),
		event.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}

	return nil
}

func (r *AuditRepository) List(
	ctx context.Context,
	filter audit.Filter,
) ([]audit.Record, int, error) {
	const baseQuery = `
		FROM audit_log
		WHERE organization_id = $1::uuid
		  AND ($2 = '' OR action = $2)
		  AND ($3 = '' OR resource_type = $3)
		  AND ($4 = '' OR resource_id = $4)
		  AND ($5 = '' OR actor_type = $5)
		  AND (
			$6 = ''
			OR action ILIKE '%' || $6 || '%'
			OR resource_type ILIKE '%' || $6 || '%'
			OR COALESCE(resource_id, '') ILIKE '%' || $6 || '%'
		  )
	`

	var total int

	if err := r.db.QueryRow(
		ctx,
		`SELECT COUNT(*) `+baseQuery,
		filter.OrganizationID,
		filter.Action,
		filter.ResourceType,
		filter.ResourceID,
		filter.ActorType,
		filter.Search,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit records: %w", err)
	}

	offset := (filter.Page - 1) * filter.PageSize

	rows, err := r.db.Query(
		ctx,
		`
		SELECT
			id::text,
			organization_id::text,
			COALESCE(actor_user_id::text, ''),
			actor_type,
			action,
			resource_type,
			COALESCE(resource_id, ''),
			COALESCE(metadata, '{}'::jsonb),
			created_at
		`+baseQuery+`
		ORDER BY created_at DESC, id DESC
		LIMIT $7
		OFFSET $8
		`,
		filter.OrganizationID,
		filter.Action,
		filter.ResourceType,
		filter.ResourceID,
		filter.ActorType,
		filter.Search,
		filter.PageSize,
		offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query audit records: %w", err)
	}
	defer rows.Close()

	records := make([]audit.Record, 0, filter.PageSize)

	for rows.Next() {
		var (
			record   audit.Record
			metadata []byte
		)

		if err := rows.Scan(
			&record.ID,
			&record.OrganizationID,
			&record.ActorUserID,
			&record.ActorType,
			&record.Action,
			&record.ResourceType,
			&record.ResourceID,
			&metadata,
			&record.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan audit record: %w", err)
		}

		if err := json.Unmarshal(metadata, &record.Metadata); err != nil {
			return nil, 0, fmt.Errorf("decode audit metadata: %w", err)
		}

		if record.Metadata == nil {
			record.Metadata = map[string]any{}
		}

		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate audit records: %w", err)
	}

	return records, total, nil
}
