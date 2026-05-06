package domain

import "time"

// OrderStatus tracks the lifecycle of an order.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusShipped   OrderStatus = "shipped"
	OrderStatusDelivered OrderStatus = "delivered"
	OrderStatusCancelled OrderStatus = "cancelled"
)

// OrderItem is a single line item in an order.
type OrderItem struct {
	ProductID  string
	Quantity   int
	PriceCents int
}

// Order represents a customer order.
type Order struct {
	ID         string
	UserID     string
	Items      []OrderItem
	Status     OrderStatus
	TotalCents int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Repository defines the interface for order persistence.
type Repository interface {
	FindByID(id string) (*Order, error)
	Save(order *Order) error
	ListByUser(userID string, limit int) ([]*Order, error)
}
