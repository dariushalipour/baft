package domain

import "time"

// User represents a user in the auth context.
type User struct {
	ID        string
	Name      string
	Email     string
	Role      UserRole
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UserRole represents the role of a user.
type UserRole string

const (
	RoleAdmin    UserRole = "admin"
	RoleMember   UserRole = "member"
	RoleViewer   UserRole = "viewer"
)

// UserRepository defines the interface for user persistence.
type UserRepository interface {
	FindByID(id string) (*User, error)
	FindByEmail(email string) (*User, error)
	Create(user *User) error
}

// Token represents an authentication token.
type Token struct {
	Value    string
	UserID   string
	Role     string
	ExpiresAt time.Time
}

// TokenRepository defines the interface for token persistence.
type TokenRepository interface {
	Save(token *Token) error
	FindByValue(value string) (*Token, error)
	Revoke(value string) error
}
