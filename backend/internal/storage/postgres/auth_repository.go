package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Namunyak2025/werstics-verify/backend/internal/auth"
)

type AuthRepository struct {
	db *pgxpool.Pool
}

func NewAuthRepository(db *pgxpool.Pool) *AuthRepository {
	return &AuthRepository{db: db}
}

func (r *AuthRepository) CreateUser(
	ctx context.Context,
	user auth.UserRepositoryRecord,
) (auth.UserRepositoryRecord, error) {
	const query = `
		INSERT INTO users (
			id,
			organization_id,
			email,
			password_hash,
			display_name,
			status,
			created_at,
			updated_at
		)
		VALUES (
			gen_random_uuid(),
			$1::uuid,
			LOWER($2),
			$3,
			$4,
			$5,
			NOW(),
			NOW()
		)
		RETURNING
			id::text,
			organization_id::text,
			email,
			password_hash,
			display_name,
			status,
			created_at,
			updated_at,
			last_login_at
	`

	var created auth.UserRepositoryRecord

	err := r.db.QueryRow(
		ctx,
		query,
		user.OrganizationID,
		user.Email,
		user.PasswordHash,
		user.DisplayName,
		user.Status,
	).Scan(
		&created.ID,
		&created.OrganizationID,
		&created.Email,
		&created.PasswordHash,
		&created.DisplayName,
		&created.Status,
		&created.CreatedAt,
		&created.UpdatedAt,
		&created.LastLoginAt,
	)

	if err != nil {
		return auth.UserRepositoryRecord{}, fmt.Errorf("create user: %w", err)
	}

	return created, nil
}

func (r *AuthRepository) FindUserByEmail(
	ctx context.Context,
	organizationID string,
	email string,
) (auth.UserRepositoryRecord, error) {
	const query = `
		SELECT
			id::text,
			organization_id::text,
			email,
			password_hash,
			display_name,
			status,
			created_at,
			updated_at,
			last_login_at
		FROM users
		WHERE organization_id = $1::uuid
		  AND LOWER(email) = LOWER($2)
		LIMIT 1
	`

	var user auth.UserRepositoryRecord

	err := r.db.QueryRow(
		ctx,
		query,
		organizationID,
		email,
	).Scan(
		&user.ID,
		&user.OrganizationID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Status,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastLoginAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return auth.UserRepositoryRecord{}, ErrNotFound
	}

	if err != nil {
		return auth.UserRepositoryRecord{}, fmt.Errorf("find user: %w", err)
	}

	return user, nil
}

func (r *AuthRepository) FindUserByID(
	ctx context.Context,
	userID string,
) (auth.UserRepositoryRecord, error) {
	const query = `
		SELECT
			id::text,
			organization_id::text,
			email,
			password_hash,
			display_name,
			status,
			created_at,
			updated_at,
			last_login_at
		FROM users
		WHERE id = $1::uuid
		LIMIT 1
	`

	var user auth.UserRepositoryRecord

	err := r.db.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.OrganizationID,
		&user.Email,
		&user.PasswordHash,
		&user.DisplayName,
		&user.Status,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastLoginAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return auth.UserRepositoryRecord{}, ErrNotFound
	}

	if err != nil {
		return auth.UserRepositoryRecord{}, fmt.Errorf("find user by id: %w", err)
	}

	return user, nil
}

func (r *AuthRepository) UpdateLastLogin(
	ctx context.Context,
	userID string,
) error {
	const query = `
		UPDATE users
		SET
			last_login_at = NOW(),
			updated_at = NOW()
		WHERE id = $1::uuid
	`

	tag, err := r.db.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("update last login: %w", err)
	}

	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}

	return nil
}

func (r *AuthRepository) CreateSession(
	ctx context.Context,
	userID string,
	tokenHash string,
	expiresAt time.Time,
) (string, error) {
	const query = `
		INSERT INTO auth_sessions (
			id,
			user_id,
			token_hash,
			expires_at
		)
		VALUES (
			gen_random_uuid(),
			$1::uuid,
			$2,
			$3
		)
		RETURNING id::text
	`

	var sessionID string

	if err := r.db.QueryRow(
		ctx,
		query,
		userID,
		tokenHash,
		expiresAt,
	).Scan(&sessionID); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	return sessionID, nil
}

func (r *AuthRepository) FindActiveSession(
	ctx context.Context,
	tokenHash string,
) (auth.Session, error) {
	const query = `
		SELECT
			id::text,
			user_id::text,
			token_hash,
			expires_at,
			revoked_at,
			created_at,
			last_seen_at
		FROM auth_sessions
		WHERE token_hash = $1
		  AND revoked_at IS NULL
		  AND expires_at > NOW()
		LIMIT 1
	`

	var session auth.Session

	err := r.db.QueryRow(
		ctx,
		query,
		tokenHash,
	).Scan(
		&session.ID,
		&session.UserID,
		&session.TokenHash,
		&session.ExpiresAt,
		&session.RevokedAt,
		&session.CreatedAt,
		&session.LastSeenAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Session{}, auth.ErrInvalidSession
	}

	if err != nil {
		return auth.Session{}, fmt.Errorf("find active session: %w", err)
	}

	return session, nil
}

func (r *AuthRepository) TouchSession(
	ctx context.Context,
	sessionID string,
) error {
	const query = `
		UPDATE auth_sessions
		SET last_seen_at = NOW()
		WHERE id = $1::uuid
		  AND revoked_at IS NULL
		  AND expires_at > NOW()
	`

	tag, err := r.db.Exec(ctx, query, sessionID)
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}

	if tag.RowsAffected() != 1 {
		return auth.ErrInvalidSession
	}

	return nil
}

func (r *AuthRepository) RevokeSession(
	ctx context.Context,
	sessionID string,
) error {
	const query = `
		UPDATE auth_sessions
		SET revoked_at = NOW()
		WHERE id = $1::uuid
		  AND revoked_at IS NULL
	`

	tag, err := r.db.Exec(ctx, query, sessionID)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	if tag.RowsAffected() != 1 {
		return auth.ErrInvalidSession
	}

	return nil
}
