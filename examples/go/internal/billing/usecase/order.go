package usecase

import (
	"github.com/example/monorepo/internal/billing/domain"
)

// CreateOrderRequest holds the input for creating an order.
type CreateOrderRequest struct {
	UserID string
	Items  []OrderItemRequest
}

// OrderItemRequest is a single line item for order creation.
type OrderItemRequest struct {
	ProductID string
	Quantity  int
}

// CreateOrderOutput is the result of creating an order.
type CreateOrderOutput struct {
	Order *domain.Order
}

// CreateOrder validates and creates a new order.
func CreateOrder(req CreateOrderRequest, repo domain.Repository) (*CreateOrderOutput, error) {
	if req.UserID == "" {
		return nil, &domain.ErrValidation{Field: "userID", Message: "required"}
	}
	if len(req.Items) == 0 {
		return nil, &domain.ErrValidation{Field: "items", Message: "at least one item required"}
	}
	for i, item := range req.Items {
		if item.ProductID == "" {
			return nil, &domain.ErrValidation{Field: "items[" + string(rune(i)) + "].productID", Message: "required"}
		}
		if item.Quantity <= 0 {
			return nil, &domain.ErrValidation{Field: "items[" + string(rune(i)) + "].quantity", Message: "must be positive"}
		}
	}

	order := &domain.Order{
		UserID: req.UserID,
		Items:  make([]domain.OrderItem, len(req.Items)),
		Status: domain.OrderStatusPending,
	}
	for i, item := range req.Items {
		order.Items[i] = domain.OrderItem{
			ProductID:  item.ProductID,
			Quantity:   item.Quantity,
			PriceCents: 0,
		}
	}

	if err := repo.Save(order); err != nil {
		return nil, err
	}

	return &CreateOrderOutput{Order: order}, nil
}

// CancelOrder cancels an existing order if it is still pending.
func CancelOrder(orderID string, repo domain.Repository) error {
	order, err := repo.FindByID(orderID)
	if err != nil {
		return err
	}
	if order.Status != domain.OrderStatusPending {
		return &domain.BillingError{Code: "order_not_pending", Message: "only pending orders can be cancelled"}
	}
	order.Status = domain.OrderStatusCancelled
	return repo.Save(order)
}
