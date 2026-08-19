package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidAmount = errors.New("amount must be greater than zero")
	ErrInvalidPaymentID = errors.New("payment id is required")
)

type Money struct {
	Currency string `json:"currency"`
	Minor    int64  `json:"minor"`
}

type Payment struct {
	ID              string        `json:"id"`
	MerchantID      string        `json:"merchant_id"`
	SessionID       string        `json:"session_id"`
	Provider        string        `json:"provider"`
	ProviderRef     string        `json:"provider_ref,omitempty"`
	Expected        Money        `json:"expected"`
	Received        *Money       `json:"received,omitempty"`
	CustomerDisplay string        `json:"customer_display,omitempty"`
	Status          PaymentStatus `json:"status"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

func (p Payment) Validate() error {
	if p.ID == "" {
		return ErrInvalidPaymentID
	}
	if p.Expected.Minor <= 0 {
		return ErrInvalidAmount
	}
	if p.Expected.Currency == "" {
		return errors.New("currency is required")
	}
	if p.MerchantID == "" {
		return errors.New("merchant id is required")
	}
	if p.Provider == "" {
		return errors.New("provider is required")
	}
	return nil
}
