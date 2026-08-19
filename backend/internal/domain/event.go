package domain

import "time"

type PaymentEvent struct {
	EventID         string    `json:"event_id"`
	Provider        string    `json:"provider"`
	ProviderEventID string    `json:"provider_event_id"`
	PaymentID       string    `json:"payment_id,omitempty"`
	ProviderRef     string    `json:"provider_ref"`
	MerchantID      string    `json:"merchant_id"`
	Amount          Money     `json:"amount"`
	CustomerDisplay string    `json:"customer_display,omitempty"`
	Kind            string    `json:"kind"`
	OccurredAt      time.Time `json:"occurred_at"`
}
