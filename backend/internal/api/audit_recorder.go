package api

import (
	"context"
	"log/slog"

	"github.com/Namunyak2025/werstics-verify/backend/internal/audit"
	"github.com/Namunyak2025/werstics-verify/backend/internal/auth"
)

type auditPermissionRecorder struct {
	service *audit.Service
}

func newAuditPermissionRecorder(
	service *audit.Service,
) *auditPermissionRecorder {
	return &auditPermissionRecorder{
		service: service,
	}
}

func (r *auditPermissionRecorder) RecordDenied(
	ctx context.Context,
	user auth.User,
	permission string,
	resourceType string,
	resourceID string,
) {
	if r == nil || r.service == nil {
		return
	}

	if err := r.service.Record(
		ctx,
		audit.Event{
			OrganizationID: user.OrganizationID,
			ActorUserID:    user.ID,
			Action:         "permission.denied",
			ResourceType:   resourceType,
			ResourceID:     resourceID,
			Metadata: map[string]any{
				"permission": permission,
			},
		},
	); err != nil {
		slog.Error(
			"audit write failed",
			"action", "permission.denied",
			"resource_type", resourceType,
			"resource_id", resourceID,
			"organization_id", user.OrganizationID,
			"actor_user_id", user.ID,
			"error", err,
		)
	}
}
