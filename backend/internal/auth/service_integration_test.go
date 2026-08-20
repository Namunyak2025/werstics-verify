package auth_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Namunyak2025/werstics-verify/backend/internal/auth"
	"github.com/Namunyak2025/werstics-verify/backend/internal/storage/postgres"
)

const authTestOrganizationID = "55555555-5555-4555-8555-555555555555"

func TestAuthenticationLifecycle(t *testing.T) {
	databaseURL := os.Getenv("WERSTICS_VERIFY_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("WERSTICS_VERIFY_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}

	ensureOrganization(t, ctx, pool)

	suffix := time.Now().UnixNano()

	email := fmt.Sprintf(
		"auth-test-%d@example.invalid",
		suffix,
	)

	password := "Strong-Test-Password-2026!"
	displayName := "Authentication Test"

	repository := postgres.NewAuthRepository(pool)
	service := auth.NewService(repository)

	user, err := service.Register(
		ctx,
		authTestOrganizationID,
		email,
		password,
		displayName,
	)
	if err != nil {
		t.Fatalf("register user: %v", err)
	}

	if user.ID == "" {
		t.Fatal("expected generated user id")
	}

	if user.OrganizationID != authTestOrganizationID {
		t.Fatalf(
			"unexpected organization id: %s",
			user.OrganizationID,
		)
	}

	if user.Email != email {
		t.Fatalf(
			"unexpected email: %s",
			user.Email,
		)
	}

	if user.DisplayName != displayName {
		t.Fatalf(
			"unexpected display name: %s",
			user.DisplayName,
		)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM users WHERE id = $1::uuid",
			user.ID,
		)
	})

	loggedIn, token, err := service.Login(
		ctx,
		authTestOrganizationID,
		email,
		password,
	)
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if token == "" {
		t.Fatal("expected non-empty session token")
	}

	if loggedIn.ID != user.ID {
		t.Fatalf(
			"expected user %s, got %s",
			user.ID,
			loggedIn.ID,
		)
	}

	authenticated, err := service.Authenticate(
		ctx,
		token,
	)
	if err != nil {
		t.Fatalf(
			"authenticate session: %v",
			err,
		)
	}

	if authenticated.ID != user.ID {
		t.Fatalf(
			"expected authenticated user %s, got %s",
			user.ID,
			authenticated.ID,
		)
	}

	if authenticated.OrganizationID != authTestOrganizationID {
		t.Fatalf(
			"expected authenticated organization %s, got %s",
			authTestOrganizationID,
			authenticated.OrganizationID,
		)
	}

	if err := service.Logout(ctx, token); err != nil {
		t.Fatalf(
			"logout: %v",
			err,
		)
	}

	if _, err := service.Authenticate(ctx, token); err != auth.ErrInvalidSession {
		t.Fatalf(
			"expected ErrInvalidSession after logout, got %v",
			err,
		)
	}
}

func TestAuthenticationRejectsWrongPassword(t *testing.T) {
	databaseURL := os.Getenv("WERSTICS_VERIFY_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("WERSTICS_VERIFY_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}

	ensureOrganization(t, ctx, pool)

	suffix := time.Now().UnixNano()

	email := fmt.Sprintf(
		"wrong-password-test-%d@example.invalid",
		suffix,
	)

	password := "Correct-Password-2026!"

	repository := postgres.NewAuthRepository(pool)
	service := auth.NewService(repository)

	user, err := service.Register(
		ctx,
		authTestOrganizationID,
		email,
		password,
		"Wrong Password Test",
	)
	if err != nil {
		t.Fatalf("register user: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM users WHERE id = $1::uuid",
			user.ID,
		)
	})

	_, _, err = service.Login(
		ctx,
		authTestOrganizationID,
		email,
		"Incorrect-Password",
	)

	if err != auth.ErrInvalidCredentials {
		t.Fatalf(
			"expected ErrInvalidCredentials, got %v",
			err,
		)
	}
}

func TestPasswordHashing(t *testing.T) {
	password := "Password-Test-2026!"

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf(
			"hash password: %v",
			err,
		)
	}

	if hash == password {
		t.Fatal("password must not be stored as plaintext")
	}

	if err := auth.VerifyPassword(password, hash); err != nil {
		t.Fatalf(
			"expected password verification to succeed: %v",
			err,
		)
	}

	if err := auth.VerifyPassword(
		"wrong-password",
		hash,
	); err != auth.ErrInvalidCredentials {
		t.Fatalf(
			"expected ErrInvalidCredentials, got %v",
			err,
		)
	}
}

func ensureOrganization(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) {
	t.Helper()

	_, err := pool.Exec(
		ctx,
		`
		INSERT INTO organizations (
			id,
			name,
			status
		)
		VALUES (
			$1::uuid,
			$2,
			'active'
		)
		ON CONFLICT (id) DO NOTHING
		`,
		authTestOrganizationID,
		"Werstics Verify Authentication Tests",
	)
	if err != nil {
		t.Fatalf(
			"create test organization: %v",
			err,
		)
	}
}
