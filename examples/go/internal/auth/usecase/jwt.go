package usecase

import (
	"time"

	"github.com/example/monorepo/internal/auth/domain"
)

// JwtService handles JWT token generation and validation.
type JwtService struct {
	userRepo  domain.UserRepository
	tokenRepo domain.TokenRepository
}

// NewJwtService creates a new JWT service.
func NewJwtService(userRepo domain.UserRepository, tokenRepo domain.TokenRepository) *JwtService {
	return &JwtService{userRepo: userRepo, tokenRepo: tokenRepo}
}

// GenerateToken generates a JWT token for the given user.
func (s *JwtService) GenerateToken(user *domain.User) (string, error) {
	token := &domain.Token{
		Value:     "fake-jwt-token-for-" + user.ID,
		UserID:    user.ID,
		Role:      string(user.Role),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := s.tokenRepo.Save(token); err != nil {
		return "", err
	}
	return token.Value, nil
}

// ValidateToken validates a JWT token and returns the user.
func (s *JwtService) ValidateToken(tokenValue string) (*domain.User, error) {
	token, err := s.tokenRepo.FindByValue(tokenValue)
	if err != nil || token == nil {
		return nil, domain.ErrUnauthorized
	}
	user, err := s.userRepo.FindByID(token.UserID)
	if err != nil {
		return nil, domain.ErrNotFound
	}
	return user, nil
}

// RequireAuth requires a valid authorization header and returns the user.
func (s *JwtService) RequireAuth(tokenValue string) (*domain.User, error) {
	if tokenValue == "" {
		return nil, domain.ErrUnauthorized
	}
	user, err := s.ValidateToken(tokenValue)
	if err != nil {
		return nil, err
	}
	return user, nil
}
