package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Namunyak2025/werstics-verify/backend/internal/audit"
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
	payments  *payments.Service
	auth      *auth.Service
	rbac      auth.PermissionChecker
	audit     *audit.Service
	permAudit *auditPermissionRecorder
}

func NewServer(
	paymentService *payments.Service,
	authService *auth.Service,
	rbacRepository auth.PermissionChecker,
	auditService *audit.Service,
) *Server {
	return &Server{
		payments:  paymentService,
		auth:      authService,
		rbac:      rbacRepository,
		audit:     auditService,
		permAudit: newAuditPermissionRecorder(auditService),
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
				s.permAudit,
				permissionPaymentCreate,
				"payment",
				http.HandlerFunc(s.paymentsHandler),
			),
		),
	)

	mux.Handle(
		"/v1/payments/",
		protected(http.HandlerFunc(s.paymentByIDHandler)),
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
		_ = s.recordAudit(
			r,
			audit.Event{
				OrganizationID: request.OrganizationID,
				Action:         "auth.registration_failed",
				ResourceType:   "user",
				Metadata: map[string]any{
					"email":  strings.ToLower(strings.TrimSpace(request.Email)),
					"reason": err.Error(),
				},
			},
		)

		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_ = s.recordAudit(
		r,
		audit.Event{
			OrganizationID: user.OrganizationID,
			Action:         "user.created",
			ResourceType:   "user",
			ResourceID:     user.ID,
			Metadata: map[string]any{
				"email":        user.Email,
				"display_name": user.DisplayName,
			},
		},
	)

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
		_ = s.recordAudit(
			r,
			audit.Event{
				OrganizationID: request.OrganizationID,
				Action:         "auth.login_failed",
				ResourceType:   "user",
				Metadata: map[string]any{
					"email":  strings.ToLower(strings.TrimSpace(request.Email)),
					"reason": err.Error(),
				},
			},
		)

		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	_ = s.recordAudit(
		r,
		audit.Event{
			OrganizationID: user.OrganizationID,
			ActorUserID:    user.ID,
			Action:         "auth.login",
			ResourceType:   "user",
			ResourceID:     user.ID,
			Metadata: map[string]any{
				"email": user.Email,
			},
		},
	)

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

	user, _ := auth.CurrentUser(r.Context())
	token := auth.BearerToken(r.Header.Get("Authorization"))

	if err := s.auth.Logout(r.Context(), token); err != nil {
		_ = s.recordAudit(
			r,
			audit.Event{
				OrganizationID: user.OrganizationID,
				ActorUserID:    user.ID,
				Action:         "auth.logout_failed",
				ResourceType:   "session",
				Metadata: map[string]any{
					"reason": err.Error(),
				},
			},
		)

		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	_ = s.recordAudit(
		r,
		audit.Event{
			OrganizationID: user.OrganizationID,
			ActorUserID:    user.ID,
			Action:         "auth.logout",
			ResourceType:   "session",
		},
	)

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

	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	var request createPaymentRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if user.OrganizationID != request.OrganizationID {
		_ = s.recordAudit(
			r,
			audit.Event{
				OrganizationID: user.OrganizationID,
				ActorUserID:    user.ID,
				Action:         "payment.create_denied",
				ResourceType:   "payment",
				ResourceID:     request.ID,
				Metadata: map[string]any{
					"reason": "organization_access_denied",
				},
			},
		)

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
		_ = s.recordAudit(
			r,
			audit.Event{
				OrganizationID: user.OrganizationID,
				ActorUserID:    user.ID,
				Action:         "payment.create_failed",
				ResourceType:   "payment",
				ResourceID:     request.ID,
				Metadata: map[string]any{
					"reason": err.Error(),
				},
			},
		)

		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	created, err := s.payments.Get(r.Context(), payment.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	_ = s.recordAudit(
		r,
		audit.Event{
			OrganizationID: user.OrganizationID,
			ActorUserID:    user.ID,
			Action:         "payment.created",
			ResourceType:   "payment",
			ResourceID:     created.ID,
			Metadata: map[string]any{
				"merchant_id": created.MerchantID,
				"provider":    created.Provider,
				"currency":    created.Expected.Currency,
				"minor":       created.Expected.Minor,
			},
		},
	)

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
			_ = s.recordAudit(
				r,
				audit.Event{
					OrganizationID: user.OrganizationID,
					ActorUserID:    user.ID,
					Action:         "payment.verification_denied",
					ResourceType:   "payment",
					ResourceID:     paymentID,
					Metadata: map[string]any{
						"reason": "organization_access_denied",
					},
				},
			)

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
			_ = s.recordAudit(
				r,
				audit.Event{
					OrganizationID: user.OrganizationID,
					ActorUserID:    user.ID,
					Action:         "payment.verification_denied",
					ResourceType:   "payment",
					ResourceID:     paymentID,
					Metadata: map[string]any{
						"reason": "permission_denied",
					},
				},
			)

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
			_ = s.recordAudit(
				r,
				audit.Event{
					OrganizationID: user.OrganizationID,
					ActorUserID:    user.ID,
					Action:         "payment.verification_failed",
					ResourceType:   "payment",
					ResourceID:     paymentID,
					Metadata: map[string]any{
						"event_id": event.EventID,
						"provider": event.Provider,
						"kind":     event.Kind,
						"reason":   err.Error(),
					},
				},
			)

			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		_ = s.recordAudit(
			r,
			audit.Event{
				OrganizationID: user.OrganizationID,
				ActorUserID:    user.ID,
				Action:         "payment.verification_completed",
				ResourceType:   "payment",
				ResourceID:     paymentID,
				Metadata: map[string]any{
					"event_id":          event.EventID,
					"provider":          event.Provider,
					"provider_event_id": event.ProviderEventID,
					"kind":              event.Kind,
					"matched":           match.Matched,
					"amount_matched":    match.AmountMatched,
					"merchant_matched":  match.MerchantMatch,
					"reason":            match.Reason,
					"status":            updated.Status,
				},
			},
		)

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
		_ = s.recordAudit(
			r,
			audit.Event{
				OrganizationID: user.OrganizationID,
				ActorUserID:    user.ID,
				Action:         "payment.read_denied",
				ResourceType:   "payment",
				ResourceID:     id,
				Metadata: map[string]any{
					"reason": "permission_denied",
				},
			},
		)

		http.Error(w, "permission denied", http.StatusForbidden)
		return
	}

	payment, err := s.payments.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "payment not found", http.StatusNotFound)
		return
	}

	if payment.OrganizationID != user.OrganizationID {
		_ = s.recordAudit(
			r,
			audit.Event{
				OrganizationID: user.OrganizationID,
				ActorUserID:    user.ID,
				Action:         "payment.read_denied",
				ResourceType:   "payment",
				ResourceID:     id,
				Metadata: map[string]any{
					"reason": "organization_access_denied",
				},
			},
		)

		http.Error(w, "organization access denied", http.StatusForbidden)
		return
	}

	writeJSON(w, http.StatusOK, payment)
}

func (s *Server) recordAudit(
	r *http.Request,
	event audit.Event,
) error {
	if s.audit == nil {
		return nil
	}

	return s.audit.Record(r.Context(), event)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
