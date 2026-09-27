package api

import (
	"io"
	"net/http"

	"github.com/Namunyak2025/werstics-verify/backend/internal/audit"
	"github.com/Namunyak2025/werstics-verify/backend/internal/providers"
	"github.com/Namunyak2025/werstics-verify/backend/internal/verification"
)

type c2BValidationResponse struct {
	ResultCode string `json:"ResultCode"`
	ResultDesc string `json:"ResultDesc"`
}

func (s *Server) c2bValidation(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		http.Error(
			w,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	body, err := io.ReadAll(
		http.MaxBytesReader(
			w,
			r.Body,
			maxProviderPayloadSize,
		),
	)
	if err != nil {
		writeJSON(
			w,
			http.StatusOK,
			c2BValidationResponse{
				ResultCode: "C2B00016",
				ResultDesc: "Rejected",
			},
		)
		return
	}

	event, err := providers.NewC2BAdapter().Normalize(
		r.Context(),
		body,
	)
	if err != nil {
		writeJSON(
			w,
			http.StatusOK,
			c2BValidationResponse{
				ResultCode: "C2B00016",
				ResultDesc: "Rejected",
			},
		)
		return
	}

	payment, err := s.payments.FindByProviderRef(
		r.Context(),
		event.Provider,
		event.ProviderRef,
		event.MerchantID,
	)
	if err != nil {
		writeJSON(
			w,
			http.StatusOK,
			c2BValidationResponse{
				ResultCode: "C2B00012",
				ResultDesc: "Rejected",
			},
		)
		return
	}

	match := verification.Match(payment, event)

	if !match.Matched {
		code := "C2B00016"

		switch match.Reason {
		case "received amount does not match expected amount":
			code = "C2B00013"
		}

		_ = s.recordAudit(
			r,
			audit.Event{
				OrganizationID: payment.OrganizationID,
				ActorType:      audit.ActorTypeSystem,
				Action:         "webhook.validation_rejected",
				ResourceType:   "payment",
				ResourceID:     payment.ID,
				Metadata: map[string]any{
					"provider":          event.Provider,
					"provider_event_id": event.ProviderEventID,
					"reason":            match.Reason,
				},
			},
		)

		writeJSON(
			w,
			http.StatusOK,
			c2BValidationResponse{
				ResultCode: code,
				ResultDesc: "Rejected",
			},
		)
		return
	}

	_ = s.recordAudit(
		r,
		audit.Event{
			OrganizationID: payment.OrganizationID,
			ActorType:      audit.ActorTypeSystem,
			Action:         "webhook.validation_accepted",
			ResourceType:   "payment",
			ResourceID:     payment.ID,
			Metadata: map[string]any{
				"provider":          event.Provider,
				"provider_event_id": event.ProviderEventID,
			},
		},
	)

	writeJSON(
		w,
		http.StatusOK,
		c2BValidationResponse{
			ResultCode: "0",
			ResultDesc: "Accepted",
		},
	)
}
