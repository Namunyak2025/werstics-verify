package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
	"github.com/Namunyak2025/werstics-verify/backend/internal/payments"
)

type Server struct {
	payments *payments.Service
}

func NewServer(paymentService *payments.Service) *Server {
	return &Server{
		payments: paymentService,
	}
}

type createPaymentRequest struct {
	ID              string       `json:"id"`
	OrganizationID  string       `json:"organization_id"`
	MerchantID      string       `json:"merchant_id"`
	SessionID       string       `json:"session_id"`
	Provider        string       `json:"provider"`
	ProviderRef     string       `json:"provider_ref,omitempty"`
	Expected        domain.Money `json:"expected"`
	CustomerDisplay string       `json:"customer_display,omitempty"`
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/v1/payments", s.paymentsHandler)
	mux.HandleFunc("/v1/payments/", s.paymentByIDHandler)

	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(
		w,
		http.StatusOK,
		map[string]string{
			"status":  "ok",
			"service": "werstics-verify",
		},
	)
}

func (s *Server) paymentsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request createPaymentRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	payment := domain.Payment{
		ID:              request.ID,
		OrganizationID:  request.OrganizationID,
		MerchantID:      request.MerchantID,
		SessionID:       request.SessionID,
		Provider:        request.Provider,
		ProviderRef:     request.ProviderRef,
		Expected:        request.Expected,
		CustomerDisplay: request.CustomerDisplay,
	}

	if err := s.payments.Create(r.Context(), payment); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	created, err := s.payments.Get(r.Context(), payment.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) paymentByIDHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/payments/")

	if id == "" {
		http.Error(w, "payment id is required", http.StatusBadRequest)
		return
	}

	if strings.HasSuffix(id, "/events") {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		paymentID := strings.TrimSuffix(id, "/events")

		var event domain.PaymentEvent

		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		event.PaymentID = paymentID

		payment, match, err := s.payments.ApplyEvent(r.Context(), event)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		writeJSON(
			w,
			http.StatusOK,
			map[string]any{
				"payment": payment,
				"match":   match,
			},
		)

		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	payment, err := s.payments.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, payment)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
