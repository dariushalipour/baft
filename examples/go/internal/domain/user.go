package domain

import "time"

// Entity is the base for all domain entities.
type Entity struct {
	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// User represents a user in the system.
type User struct {
	Entity
	Name  string
	Email string
}
