package providers

import (
	"context"
	"errors"
	"fmt"

	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
)

var (
	ErrUnsupportedProvider = errors.New("unsupported payment provider")
	ErrInvalidSignature    = errors.New("invalid provider signature")
	ErrMalformedPayload    = errors.New("malformed provider payload")
)

type Adapter interface {
	Name() string

	VerifySignature(
		ctx context.Context,
		payload []byte,
		headers map[string]string,
	) error

	Normalize(
		ctx context.Context,
		payload []byte,
	) (domain.PaymentEvent, error)
}

type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry(adapters ...Adapter) *Registry {
	registry := &Registry{
		adapters: make(map[string]Adapter),
	}

	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}

		registry.adapters[adapter.Name()] = adapter
	}

	return registry
}

func (r *Registry) Get(name string) (Adapter, error) {
	adapter, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf(
			"%w: %s",
			ErrUnsupportedProvider,
			name,
		)
	}

	return adapter, nil
}
