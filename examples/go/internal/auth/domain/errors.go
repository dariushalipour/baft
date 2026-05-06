package domain

// AuthError represents an authentication error.
type AuthError struct {
	Code    string
	Message string
}

func (e *AuthError) Error() string {
	return "auth[" + e.Code + "]: " + e.Message
}

// ErrUnauthorized is returned when authentication fails.
var ErrUnauthorized = &AuthError{Code: "unauthorized", Message: "authentication required"}

// ErrNotFound is returned when a resource does not exist.
var ErrNotFound = &AuthError{Code: "not_found", Message: "resource not found"}
