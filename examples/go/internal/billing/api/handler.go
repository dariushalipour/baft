package api

import (
	"encoding/json"
	"net/http"

	"github.com/example/monorepo/internal/billing/usecase"
)

// Handler handles HTTP requests for the billing API.
type Handler struct{}

// NewHandler creates a new billing handler.
func NewHandler() *Handler {
	return &Handler{}
}

// CreateOrderHandler handles POST /orders.
func (h *Handler) CreateOrderHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string                  `json:"user_id"`
		Items  []usecase.OrderItemRequest `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	out, err := usecase.CreateOrder(usecase.CreateOrderRequest{
		UserID: req.UserID,
		Items:  req.Items,
	}, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(out)
}

// CancelOrderHandler handles POST /orders/{id}/cancel.
func (h *Handler) CancelOrderHandler(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("id")
	if err := usecase.CancelOrder(orderID, nil); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
