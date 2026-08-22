package api

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Namunyak2025/werstics-verify/backend/internal/audit"
	"github.com/Namunyak2025/werstics-verify/backend/internal/providers"
)

const maxProviderPayloadSize = 1 << 20

func (s *Server) providerWebhook(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.ingestion == nil {
		http.Error(
			w,
			"provider ingestion unavailable",
			http.StatusServiceUnavailable,
		)
		return
	}

	provider := strings.TrimPrefix(r.URL.Path, "/v1/providers/")
	provider = strings.TrimSuffix(provider, "/webhook")
	provider = strings.TrimSpace(provider)

	if provider == "" || strings.Contains(provider, "/") {
		http.Error(w, "provider is required", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(
		http.MaxBytesReader(w, r.Body, maxProviderPayloadSize),
	)
	if err != nil {
		slog.Error(
			"provider webhook payload read failed",
			"provider", provider,
			"error", err,
		)

		http.Error(
			w,
			"invalid provider payload",
			http.StatusBadRequest,
		)
		return
	}

	headers := make(map[string]string)

	for key, values := range r.Header {
		if len(values) == 0 {
			continue
		}

		headers[key] = values[0]
	}

	payment, err := s.ingestion.Ingest(
		r.Context(),
		provider,
		body,
		headers,
	)
	if err != nil {
		slog.Warn(
			"provider webhook rejected",
			"provider", provider,
			"error", err,
		)

		status := http.StatusBadRequest

		switch {
		case errors.Is(err, providers.ErrUnsupportedProvider):
			status = http.StatusNotFound

		case errors.Is(err, providers.ErrInvalidSignature),
			strings.Contains(err.Error(), "verify provider signature"):
			status = http.StatusUnauthorized

		case errors.Is(err, providers.ErrMalformedPayload),
			strings.Contains(err.Error(), "normalize provider event"):
			status = http.StatusBadRequest
		}

		_ = s.recordAudit(
			r,
			audit.Event{
				OrganizationID: payment.OrganizationID,
				ActorType:      audit.ActorTypeSystem,
				Action:         "webhook.rejected",
				ResourceType:   "provider_webhook",
				ResourceID:     provider,
				Metadata: map[string]any{
					"provider": provider,
					"reason":   err.Error(),
				},
			},
		)

		http.Error(
			w,
			"provider webhook rejected",
			status,
		)
		return
	}

	_ = s.recordAudit(
		r,
		audit.Event{
			OrganizationID: payment.OrganizationID,
			ActorType:      audit.ActorTypeSystem,
			Action:         "webhook.accepted",
			ResourceType:   "payment",
			ResourceID:     payment.ID,
			Metadata: map[string]any{
				"provider": payment.Provider,
			},
		},
	)

	writeJSON(
		w,
		http.StatusOK,
		map[string]any{
			"status":     "accepted",
			"payment_id": payment.ID,
		},
	)
}
