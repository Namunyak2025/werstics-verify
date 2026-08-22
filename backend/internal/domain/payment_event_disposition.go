package domain

import "errors"

type PaymentEventDisposition string

const (
	EventDispositionAccepted  PaymentEventDisposition = "accepted"
	EventDispositionDuplicate PaymentEventDisposition = "duplicate"
	EventDispositionConflict  PaymentEventDisposition = "conflict"
)

var ErrPaymentEventConflict = errors.New("conflicting payment event")
