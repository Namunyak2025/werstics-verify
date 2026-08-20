package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Namunyak2025/werstics-verify/backend/internal/auth"
	"github.com/Namunyak2025/werstics-verify/backend/internal/domain"
	"github.com/Namunyak2025/werstics-verify/backend/internal/payments"
)

const (
	permissionPaymentCreate = "payment:create"
	permissionPaymentRead   = "payment:read"
	permissionPaymentVerify = "payment:verify"
)

type Server struct {
	payments *payments.Service
	auth     *auth.Service
	rbac     auth.PermissionChecker
}

func NewServer(
	paymentService *payments.Service,
	authService *auth.Service,
	rbacRepository auth.PermissionChecker,
) *Server {
	return &Server{
		payments: paymentService,
		auth:     authService,
		rbac:     rbacRepository,
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

type registerRequest struct {
	OrganizationID string `json:"organization_id"`
	Email          string `json:"email"`
	Password       string `json:"password"`
	DisplayName    string `json:"display_name"`
}

type loginRequest struct {
	OrganizationID string `json:"organization_id"`
	Email          string `json:"email"`
	Password       string `json:"password"`
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", s.health)

	mux.HandleFunc("/v1/auth/register", s.register)
	mux.HandleFunc("/v1/auth/login", s.login)

	protected := auth.Middleware(s.auth)

	mux.Handle(
		"/v1/auth/logout",
		protected(http.HandlerFunc(s.logout)),
	)

	mux.Handle(
		"/v1/auth/me",
		protected(http.HandlerFunc(s.me)),
	)

	mux.Handle(
		"/v1/payments",
		protected(
			auth.RequirePermission(
				s.rbac,
				permissionPaymentCreate,
				http.HandlerFunc(s.paymentsHandler),
			),
		),
	)

	mux.Handle(
		"/v1/payments/",
		protected(
			http.HandlerFunc(s.paymentByIDHandler),
		),
	)

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

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request registerRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	user, err := s.auth.Register(
		r.Context(),
		request.OrganizationID,
		request.Email,
		request.Password,
		request.DisplayName,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(
		w,
		http.StatusCreated,
		map[string]any{
			"user": user,
		},
	)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request loginRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	user, token, err := s.auth.Login(
		r.Context(),
		request.OrganizationID,
		request.Email,
		request.Password,
	)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		map[string]any{
			"token": token,
			"user":  user,
		},
	)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := auth.BearerToken(r.Header.Get("Authorization"))

	if err := s.auth.Logout(r.Context(), token); err != nil {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		map[string]string{
			"status": "logged_out",
		},
	)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	permissions, err := auth.CurrentPermissions(
		r.Context(),
		s.rbac,
	)
	if err != nil {
		http.Error(w, "authorization check failed", http.StatusInternalServerError)
		return
	}

	writeJSON(
		w,
		http.StatusOK,
		map[string]any{
			"user":        user,
			"permissions": permissions,
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

	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	if user.OrganizationID != request.OrganizationID {
		http.Error(w, "organization access denied", http.StatusForbidden)
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

	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	if strings.HasSuffix(id, "/events") {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		paymentID := strings.TrimSuffix(id, "/events")

		payment, err := s.payments.Get(
			r.Context(),
			paymentID,
		)
		if err != nil {
			http.Error(w, "payment not found", http.StatusNotFound)
			return
		}

		if payment.OrganizationID != user.OrganizationID {
			http.Error(w, "organization access denied", http.StatusForbidden)
			return
		}

		allowed, err := s.rbac.HasPermission(
			r.Context(),
			user.ID,
			permissionPaymentVerify,
		)
		if err != nil {
			http.Error(w, "authorization check failed", http.StatusInternalServerError)
			return
		}

		if !allowed {
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		}

		var event domain.PaymentEvent

		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		event.PaymentID = paymentID

		updated, match, err := s.payments.ApplyEvent(
			r.Context(),
			event,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		writeJSON(
			w,
			http.StatusOK,
			map[string]any{
				"payment": updated,
				"match":   match,
			},
		)

		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	allowed, err := s.rbac.HasPermission(
		r.Context(),
		user.ID,
		permissionPaymentRead,
	)
	if err != nil {
		http.Error(w, "authorization check failed", http.StatusInternalServerError)
		return
	}

	if !allowed {
		http.Error(w, "permission denied", http.StatusForbidden)
		return
	}

	payment, err := s.payments.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "payment not found", http.StatusNotFound)
		return
	}

	if payment.OrganizationID != user.OrganizationID {
		http.Error(w, "organization access denied", http.StatusForbidden)
		return
	}

	writeJSON(w, http.StatusOK, payment)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
