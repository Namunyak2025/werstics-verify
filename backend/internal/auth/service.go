package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const DefaultSessionLifetime = 24 * time.Hour

var (
	ErrInvalidOrganization = errors.New("organization id is required")
	ErrInvalidEmail        = errors.New("email is required")
	ErrInvalidDisplayName  = errors.New("display name is required")
	ErrInactiveUser        = errors.New("user is not active")
)

type User struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Email          string `json:"email"`
	DisplayName    string `json:"display_name"`
	Status         string `json:"status"`
}

type UserRepositoryRecord struct {
	ID             string
	OrganizationID string
	Email          string
	PasswordHash   string
	DisplayName    string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastLoginAt    *time.Time
}

type Repository interface {
	CreateUser(ctx context.Context, user UserRepositoryRecord) (UserRepositoryRecord, error)
	FindUserByEmail(ctx context.Context, organizationID, email string) (UserRepositoryRecord, error)
	FindUserByID(ctx context.Context, userID string) (UserRepositoryRecord, error)
	UpdateLastLogin(ctx context.Context, userID string) error
	CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (string, error)
	FindActiveSession(ctx context.Context, tokenHash string) (Session, error)
	TouchSession(ctx context.Context, sessionID string) error
	RevokeSession(ctx context.Context, sessionID string) error
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Register(
	ctx context.Context,
	organizationID string,
	email string,
	password string,
	displayName string,
) (User, error) {
	organizationID = strings.TrimSpace(organizationID)
	email = strings.ToLower(strings.TrimSpace(email))
	displayName = strings.TrimSpace(displayName)

	if organizationID == "" {
		return User{}, ErrInvalidOrganization
	}

	if email == "" {
		return User{}, ErrInvalidEmail
	}

	if displayName == "" {
		return User{}, ErrInvalidDisplayName
	}

	passwordHash, err := HashPassword(password)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}

	record, err := s.repo.CreateUser(ctx, UserRepositoryRecord{
		OrganizationID: organizationID,
		Email:          email,
		PasswordHash:   passwordHash,
		DisplayName:    displayName,
		Status:         "active",
	})
	if err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}

	return userFromRecord(record), nil
}

func (s *Service) Login(
	ctx context.Context,
	organizationID string,
	email string,
	password string,
) (User, string, error) {
	organizationID = strings.TrimSpace(organizationID)
	email = strings.ToLower(strings.TrimSpace(email))

	if organizationID == "" || email == "" || password == "" {
		return User{}, "", ErrInvalidCredentials
	}

	record, err := s.repo.FindUserByEmail(ctx, organizationID, email)
	if err != nil {
		return User{}, "", ErrInvalidCredentials
	}

	if record.Status != "active" {
		return User{}, "", ErrInactiveUser
	}

	if err := VerifyPassword(password, record.PasswordHash); err != nil {
		return User{}, "", ErrInvalidCredentials
	}

	token, tokenHash, err := GenerateSessionToken()
	if err != nil {
		return User{}, "", fmt.Errorf("generate session token: %w", err)
	}

	if _, err := s.repo.CreateSession(
		ctx,
		record.ID,
		tokenHash,
		time.Now().UTC().Add(DefaultSessionLifetime),
	); err != nil {
		return User{}, "", fmt.Errorf("create authentication session: %w", err)
	}

	if err := s.repo.UpdateLastLogin(ctx, record.ID); err != nil {
		return User{}, "", fmt.Errorf("update last login: %w", err)
	}

	return userFromRecord(record), token, nil
}

func (s *Service) Authenticate(
	ctx context.Context,
	token string,
) (User, error) {
	token = strings.TrimSpace(token)

	if token == "" {
		return User{}, ErrInvalidSession
	}

	session, err := s.repo.FindActiveSession(
		ctx,
		HashSessionToken(token),
	)
	if err != nil {
		return User{}, ErrInvalidSession
	}

	record, err := s.repo.FindUserByID(ctx, session.UserID)
	if err != nil {
		return User{}, ErrInvalidSession
	}

	if record.Status != "active" {
		return User{}, ErrInvalidSession
	}

	if err := s.repo.TouchSession(ctx, session.ID); err != nil {
		return User{}, ErrInvalidSession
	}

	return userFromRecord(record), nil
}

func (s *Service) Logout(
	ctx context.Context,
	token string,
) error {
	token = strings.TrimSpace(token)

	if token == "" {
		return ErrInvalidSession
	}

	session, err := s.repo.FindActiveSession(
		ctx,
		HashSessionToken(token),
	)
	if err != nil {
		return ErrInvalidSession
	}

	return s.repo.RevokeSession(ctx, session.ID)
}

func userFromRecord(record UserRepositoryRecord) User {
	return User{
		ID:             record.ID,
		OrganizationID: record.OrganizationID,
		Email:          record.Email,
		DisplayName:    record.DisplayName,
		Status:         record.Status,
	}
}
