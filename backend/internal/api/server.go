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

func NewServer() *Server {
	return &Server{payments: payments.NewService()}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/v1/payments", s.paymentsHandler)
	mux.HandleFunc("/v1/payments/", s.paymentByIDHandler)
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "werstics-verify"})
}

func (s *Server) paymentsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var p domain.Payment
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := s.payments.Create(p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) paymentByIDHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/payments/")
	if r.Method == http.MethodGet {
		p, ok := s.payments.Get(id)
		if !ok {
			http.Error(w, "payment not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, p)
		return
	}
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/events") {
		id = strings.TrimSuffix(id, "/events")
		var event domain.PaymentEvent
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		event.PaymentID = id
		p, match, err := s.payments.ApplyEvent(event)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"payment": p, "match": match})
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
