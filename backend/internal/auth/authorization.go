package auth

import (
	"context"
	"errors"
	"net/http"
)

type PermissionChecker interface {
	HasPermission(ctx context.Context, userID string, permission string) (bool, error)
	ListPermissions(ctx context.Context, userID string) ([]string, error)
}

var ErrForbidden = errors.New("forbidden")

func RequirePermission(
	checker PermissionChecker,
	permission string,
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := CurrentUser(r.Context())
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer`)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}

		allowed, err := checker.HasPermission(
			r.Context(),
			user.ID,
			permission,
		)
		if err != nil {
			http.Error(w, "authorization check failed", http.StatusInternalServerError)
			return
		}

		if !allowed {
			http.Error(w, "permission denied", http.StatusForbidden)
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
