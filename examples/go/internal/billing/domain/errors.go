package domain

// BillingError represents a billing-related error.
type BillingError struct {
	Code    string
	Message string
}

func (e *BillingError) Error() string {
	return "billing[" + e.Code + "]: " + e.Message
}

// ErrNotFound is returned when an order does not exist.
var ErrNotFound = &BillingError{Code: "not_found", Message: "order not found"}

// ErrConflict is returned when an order is in an invalid state.
var ErrConflict = &BillingError{Code: "conflict", Message: "order conflict"}

// ErrValidation is returned when input validation fails.
type ErrValidation struct {
	Field   string
	Message string
}

func (e *ErrValidation) Error() string {
	return e.Field + ": " + e.Message
}
