package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type RBACRepository struct {
	db *pgxpool.Pool
}

func NewRBACRepository(db *pgxpool.Pool) *RBACRepository {
	return &RBACRepository{db: db}
}

func (r *RBACRepository) HasPermission(
	ctx context.Context,
	userID string,
	permission string,
) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM user_roles ur
			JOIN roles ro
				ON ro.id = ur.role_id
			JOIN role_permissions rp
				ON rp.role_id = ro.id
			JOIN permissions pe
				ON pe.id = rp.permission_id
			JOIN users u
				ON u.id = ur.user_id
			   AND u.organization_id = ro.organization_id
			WHERE ur.user_id = $1::uuid
			  AND u.status = 'active'
			  AND pe.name = $2
		)
	`

	var allowed bool

	if err := r.db.QueryRow(
		ctx,
		query,
		userID,
		permission,
	).Scan(&allowed); err != nil {
		return false, fmt.Errorf("check permission: %w", err)
	}

	return allowed, nil
}

func (r *RBACRepository) ListPermissions(
	ctx context.Context,
	userID string,
) ([]string, error) {
	const query = `
		SELECT DISTINCT pe.name
		FROM user_roles ur
		JOIN roles ro
			ON ro.id = ur.role_id
		JOIN role_permissions rp
			ON rp.role_id = ro.id
		JOIN permissions pe
			ON pe.id = rp.permission_id
		JOIN users u
			ON u.id = ur.user_id
		   AND u.organization_id = ro.organization_id
		WHERE ur.user_id = $1::uuid
		  AND u.status = 'active'
		ORDER BY pe.name
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	defer rows.Close()

	var permissions []string

	for rows.Next() {
		var permission string

		if err := rows.Scan(&permission); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}

		permissions = append(permissions, permission)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate permissions: %w", err)
	}

	return permissions, nil
}
