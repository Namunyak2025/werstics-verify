package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Namunyak2025/werstics-verify/backend/internal/audit"
	"github.com/Namunyak2025/werstics-verify/backend/internal/auth"
)

func (s *Server) providerFailuresHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	allowed, err := s.rbac.HasPermission(
		r.Context(),
		user.ID,
		permissionPaymentVerify,
	)
	if err != nil {
		http.Error(w, "authorization check failed", http.StatusInternalServerError)
		return
	}

	if !allowed {
		_ = s.recordAudit(
			r,
			audit.Event{
				OrganizationID: user.OrganizationID,
				ActorUserID:    user.ID,
				Action:         "provider_failure.list_denied",
				ResourceType:   "provider_event_failure",
				Metadata: map[string]any{
					"reason": "permission_denied",
				},
			},
		)

		http.Error(w, "permission denied", http.StatusForbidden)
		return
	}

	if s.failures == nil {
		http.Error(
			w,
			"provider failure repository unavailable",
			http.StatusServiceUnavailable,
		)
		return
	}

	page := 1
	pageSize := 25

	if raw := r.URL.Query().Get("page"); raw != "" {
		if value, parseErr := strconv.Atoi(raw); parseErr == nil && value > 0 {
			page = value
		}
	}

	if raw := r.URL.Query().Get("page_size"); raw != "" {
		if value, parseErr := strconv.Atoi(raw); parseErr == nil && value > 0 {
			pageSize = value
		}
	}

	status := strings.TrimSpace(r.URL.Query().Get("status"))

	items, total, err := s.failures.List(
		r.Context(),
		user.OrganizationID,
		status,
		page,
		pageSize,
	)
	if err != nil {
		http.Error(
			w,
			"failed to list provider failures",
			http.StatusInternalServerError,
		)
		return
	}

	_ = s.recordAudit(
		r,
		audit.Event{
			OrganizationID: user.OrganizationID,
			ActorUserID:    user.ID,
			Action:         "provider_failure.list",
			ResourceType:   "provider_event_failure",
			Metadata: map[string]any{
				"status":    status,
				"page":      page,
				"page_size": pageSize,
				"total":     total,
			},
		},
	)

	writeJSON(
		w,
		http.StatusOK,
		map[string]any{
			"failures":  items,
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	)
}

func (s *Server) providerFailureActionHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	allowed, err := s.rbac.HasPermission(
		r.Context(),
		user.ID,
		permissionPaymentVerify,
	)
	if err != nil {
		http.Error(w, "authorization check failed", http.StatusInternalServerError)
		return
	}

	if !allowed {
		_ = s.recordAudit(
			r,
			audit.Event{
				OrganizationID: user.OrganizationID,
				ActorUserID:    user.ID,
				Action:         "provider_failure.action_denied",
				ResourceType:   "provider_event_failure",
				Metadata: map[string]any{
					"reason": "permission_denied",
				},
			},
		)

		http.Error(w, "permission denied", http.StatusForbidden)
		return
	}

	if s.failures == nil || s.ingestion == nil {
		http.Error(
			w,
			"provider recovery unavailable",
			http.StatusServiceUnavailable,
		)
		return
	}

	path := strings.TrimPrefix(
		r.URL.Path,
		"/v1/provider-failures/",
	)
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) != 2 || parts[0] == "" {
		http.Error(w, "invalid provider failure path", http.StatusBadRequest)
		return
	}

	id := parts[0]
	action := parts[1]

	switch action {
	case "retry":
		s.retryProviderFailure(w, r, user, id)

	case "resolve":
		s.resolveProviderFailure(w, r, user, id)

	default:
		http.Error(w, "unsupported provider failure action", http.StatusNotFound)
	}
}

func (s *Server) retryProviderFailure(
	w http.ResponseWriter,
	r *http.Request,
	user auth.User,
	id string,
) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	result, err := s.ingestion.RetryFailure(
		r.Context(),
		id,
		user.OrganizationID,
	)
	if err != nil {
		_ = s.recordAudit(
			r,
			audit.Event{
				OrganizationID: user.OrganizationID,
				ActorUserID:    user.ID,
				Action:         "provider_failure.retry_failed",
				ResourceType:   "provider_event_failure",
				ResourceID:     id,
				Metadata: map[string]any{
					"reason": err.Error(),
				},
			},
		)

		if strings.Contains(err.Error(), "record not found") {
			http.Error(w, "provider failure not found", http.StatusNotFound)
			return
		}

		http.Error(
			w,
			"provider failure retry failed",
			http.StatusInternalServerError,
		)
		return
	}

	_ = s.recordAudit(
		r,
		audit.Event{
			OrganizationID: user.OrganizationID,
			ActorUserID:    user.ID,
			Action:         "provider_failure.retry",
			ResourceType:   "provider_event_failure",
			ResourceID:     id,
			Metadata: map[string]any{
				"payment_id": result.Payment.ID,
			},
		},
	)

	writeJSON(
		w,
		http.StatusOK,
		map[string]any{
			"status":     "resolved",
			"payment_id": result.Payment.ID,
		},
	)
}

func (s *Server) resolveProviderFailure(
	w http.ResponseWriter,
	r *http.Request,
	user auth.User,
	id string,
) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := s.failures.Resolve(
		r.Context(),
		id,
		user.OrganizationID,
	)
	if err != nil {
		_ = s.recordAudit(
			r,
			audit.Event{
				OrganizationID: user.OrganizationID,
				ActorUserID:    user.ID,
				Action:         "provider_failure.resolve_failed",
				ResourceType:   "provider_event_failure",
				ResourceID:     id,
				Metadata: map[string]any{
					"reason": err.Error(),
				},
			},
		)

		if strings.Contains(err.Error(), "record not found") {
			http.Error(w, "provider failure not found", http.StatusNotFound)
			return
		}

		http.Error(
			w,
			"provider failure resolve failed",
			http.StatusInternalServerError,
		)
		return
	}

	_ = s.recordAudit(
		r,
		audit.Event{
			OrganizationID: user.OrganizationID,
			ActorUserID:    user.ID,
			Action:         "provider_failure.resolve",
			ResourceType:   "provider_event_failure",
			ResourceID:     id,
		},
	)

	writeJSON(
		w,
		http.StatusOK,
		map[string]string{
			"status": "resolved",
		},
	)
}
