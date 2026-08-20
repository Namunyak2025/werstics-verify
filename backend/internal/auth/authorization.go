package auth

import (
	"context"
	"errors"
	"net/http"
)

var ErrForbidden = errors.New("forbidden")

type PermissionChecker interface {
	HasPermission(
		ctx context.Context,
		userID string,
		permission string,
	) (bool, error)

	ListPermissions(
		ctx context.Context,
		userID string,
	) ([]string, error)
}

type AuditRecorder interface {
	RecordDenied(
		ctx context.Context,
		user User,
		permission string,
		resourceType string,
		resourceID string,
	)
}

func RequirePermission(
	checker PermissionChecker,
	recorder AuditRecorder,
	permission string,
	resourceType string,
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := CurrentUser(r.Context())
		if !ok {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(
				w,
				"authentication required",
				http.StatusUnauthorized,
			)
			return
		}

		allowed, err := checker.HasPermission(
			r.Context(),
			user.ID,
			permission,
		)
		if err != nil {
			http.Error(
				w,
				"authorization check failed",
				http.StatusInternalServerError,
			)
			return
		}

		if !allowed {
			if recorder != nil {
				recorder.RecordDenied(
					r.Context(),
					user,
					permission,
					resourceType,
					r.URL.Path,
				)
			}

			http.Error(
				w,
				"permission denied",
				http.StatusForbidden,
			)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func CurrentPermissions(
	ctx context.Context,
	checker PermissionChecker,
) ([]string, error) {
	user, ok := CurrentUser(ctx)
	if !ok {
		return nil, ErrForbidden
	}

	return checker.ListPermissions(ctx, user.ID)
}
