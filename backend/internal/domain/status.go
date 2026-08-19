package domain

import "fmt"

type PaymentStatus string

const (
	StatusRequested PaymentStatus = "requested"
	StatusPending   PaymentStatus = "pending"
	StatusConfirmed PaymentStatus = "confirmed"
	StatusSettled   PaymentStatus = "settled"
	StatusFailed    PaymentStatus = "failed"
	StatusExpired   PaymentStatus = "expired"
	StatusReversed  PaymentStatus = "reversed"
	StatusRefunded  PaymentStatus = "refunded"
	StatusCancelled PaymentStatus = "cancelled"
)

func (s PaymentStatus) String() string { return string(s) }

func (s PaymentStatus) Terminal() bool {
	switch s {
	case StatusSettled, StatusFailed, StatusExpired, StatusReversed, StatusRefunded, StatusCancelled:
		return true
	default:
		return false
	}
}

func CanTransition(from, to PaymentStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case StatusRequested:
		return to == StatusPending || to == StatusCancelled || to == StatusExpired
	case StatusPending:
		return to == StatusConfirmed || to == StatusFailed || to == StatusExpired || to == StatusCancelled
	case StatusConfirmed:
		return to == StatusSettled || to == StatusReversed || to == StatusRefunded
	case StatusSettled:
		return to == StatusReversed || to == StatusRefunded
	case StatusFailed, StatusExpired, StatusCancelled:
		return false
	case StatusReversed:
		return to == StatusRefunded
	case StatusRefunded:
		return false
	default:
		return false
	}
}

func ValidateTransition(from, to PaymentStatus) error {
	if !CanTransition(from, to) {
		return fmt.Errorf("invalid payment status transition: %s -> %s", from, to)
	}
	return nil
}
