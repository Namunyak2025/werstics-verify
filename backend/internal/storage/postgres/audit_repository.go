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
			NULLIF($5, ''),
			$6::jsonb,
			$7
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
