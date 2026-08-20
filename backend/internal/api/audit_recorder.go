package api

import (
	"context"

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

	_ = r.service.Record(
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
	)
}
