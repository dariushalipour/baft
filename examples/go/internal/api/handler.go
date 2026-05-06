package api

import (
	"encoding/json"
	"net/http"

	authusecase "github.com/example/monorepo/internal/auth/usecase"
	billingapi "github.com/example/monorepo/internal/billing/api"
	billingusecase "github.com/example/monorepo/internal/billing/usecase"
)

// Handler handles HTTP requests at the root level.
type Handler struct {
	BillingHandler *billingapi.Handler
	JwtService     *authusecase.JwtService
}

// NewHandler creates a new root handler.
func NewHandler(jwtService *authusecase.JwtService) *Handler {
	return &Handler{
		BillingHandler: billingapi.NewHandler(),
		JwtService:     jwtService,
	}
}

// RegisterRoutes registers all routes.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /orders", h.CreateOrderHandler)
	mux.HandleFunc("POST /orders/{id}/cancel", h.CancelOrderHandler)
}

// CreateOrderHandler handles POST /orders.
func (h *Handler) CreateOrderHandler(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	if token != "" {
		token = token[7:] // strip "Bearer "
	}
	_, err := h.JwtService.RequireAuth(token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		UserID string `json:"user_id"`
		Items  []struct {
			ProductID string `json:"product_id"`
			Quantity  int    `json:"quantity"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	items := make([]billingusecase.OrderItemRequest, len(req.Items))
	for i, item := range req.Items {
		items[i] = billingusecase.OrderItemRequest{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
		}
	}

	_, err = billingusecase.CreateOrder(billingusecase.CreateOrderRequest{
		UserID: req.UserID,
		Items:  items,
	}, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// CancelOrderHandler handles POST /orders/{id}/cancel.
func (h *Handler) CancelOrderHandler(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	if token != "" {
		token = token[7:] // strip "Bearer "
	}
	_, err := h.JwtService.RequireAuth(token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	orderID := r.PathValue("id")
	if err := billingusecase.CancelOrder(orderID, nil); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
