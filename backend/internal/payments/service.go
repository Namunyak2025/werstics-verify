package payments

import (
	"fmt"
	"sync"
	"time"

	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
	"github.com/Namunyak2025/werstics-verify/backend/internal/verification"
)

type Service struct {
	mu       sync.RWMutex
	payments map[string]domain.Payment
}

func NewService() *Service {
	return &Service{payments: make(map[string]domain.Payment)}
}

func (s *Service) Create(p domain.Payment) error {
	if err := p.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	p.Status = domain.StatusRequested
	p.CreatedAt = now
	p.UpdatedAt = now

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.payments[p.ID]; exists {
		return fmt.Errorf("payment %q already exists", p.ID)
	}
	s.payments[p.ID] = p
	return nil
}

func (s *Service) Get(id string) (domain.Payment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.payments[id]
	return p, ok
}

func (s *Service) ApplyEvent(event domain.PaymentEvent) (domain.Payment, verification.MatchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	payment, ok := s.payments[event.PaymentID]
	if !ok {
		return domain.Payment{}, verification.MatchResult{}, fmt.Errorf("payment %q not found", event.PaymentID)
	}

	match := verification.Match(payment, event)
	if !match.Matched {
		return payment, match, nil
	}

	var target domain.PaymentStatus
	switch event.Kind {
	case "payment.pending":
		target = domain.StatusPending
	case "payment.confirmed":
		target = domain.StatusConfirmed
	case "payment.settled":
		target = domain.StatusSettled
	case "payment.failed":
		target = domain.StatusFailed
	case "payment.expired":
		target = domain.StatusExpired
	case "payment.reversed":
		target = domain.StatusReversed
	case "payment.refunded":
		target = domain.StatusRefunded
	case "payment.cancelled":
		target = domain.StatusCancelled
	default:
		return payment, match, fmt.Errorf("unsupported event kind %q", event.Kind)
	}

	if err := domain.ValidateTransition(payment.Status, target); err != nil {
		return payment, match, err
	}

	received := event.Amount
	payment.Received = &received
	payment.ProviderRef = event.ProviderRef
	payment.CustomerDisplay = event.CustomerDisplay
	payment.Status = target
	payment.UpdatedAt = time.Now().UTC()
	s.payments[payment.ID] = payment

	return payment, match, nil
}
