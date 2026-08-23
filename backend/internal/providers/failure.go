package providers

import (
	"context"
	"time"
)

type EventFailure struct {
	ID              string
	Provider        string
	ProviderEventID string
	EventID         string
	PaymentID       string
	ProviderRef     string
	MerchantID      string
	AmountCurrency  string
	AmountMinor     int64
	CustomerDisplay string
	Kind            string
	OccurredAt      time.Time
	Status          string
	Attempts        int
	LastError       string
	FirstFailedAt   time.Time
	LastFailedAt    time.Time
	ResolvedAt      *time.Time
}

type FailureRepository interface {
	Record(context.Context, EventFailure) error
	Get(context.Context, string, string) (EventFailure, error)
	List(context.Context, string, string, int, int) ([]EventFailure, int, error)
	Resolve(context.Context, string, string) error
}
