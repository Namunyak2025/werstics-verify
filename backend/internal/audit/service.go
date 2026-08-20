package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	ActorTypeUser   = "user"
	ActorTypeSystem = "system"
)

var ErrInvalidActorType = errors.New("invalid audit actor type")

type Event struct {
	OrganizationID string
	ActorUserID    string
	ActorType      string
	Action         string
	ResourceType   string
	ResourceID     string
	Metadata       map[string]any
	CreatedAt      time.Time
}

type Repository interface {
	Record(ctx context.Context, event Event) error
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Record(
	ctx context.Context,
	event Event,
) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}

	if event.Metadata == nil {
		event.Metadata = map[string]any{}
	}

	if event.ActorType == "" {
		if event.ActorUserID == "" {
			event.ActorType = ActorTypeSystem
		} else {
			event.ActorType = ActorTypeUser
		}
	}

	if event.ActorType != ActorTypeUser &&
		event.ActorType != ActorTypeSystem {
		return ErrInvalidActorType
	}

	if event.ActorType == ActorTypeSystem {
		event.ActorUserID = ""
	}

	if _, err := json.Marshal(event.Metadata); err != nil {
		return fmt.Errorf("validate audit metadata: %w", err)
	}

	if err := s.repo.Record(ctx, event); err != nil {
		return fmt.Errorf("record audit event: %w", err)
	}

	return nil
}
