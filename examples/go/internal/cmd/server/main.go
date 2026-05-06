package main

import (
	"log"
	"net/http"

	"github.com/example/monorepo/internal/api"
	authdomain "github.com/example/monorepo/internal/auth/domain"
	authusecase "github.com/example/monorepo/internal/auth/usecase"
)

func main() {
	mux := http.NewServeMux()

	// In a real app, these would be wired up with DI
	var userRepo authDomain.UserRepository
	var tokenRepo authDomain.TokenRepository
	jwtService := authusecase.NewJwtService(userRepo, tokenRepo)
	handler := api.NewHandler(jwtService)
	handler.RegisterRoutes(mux)

	log.Println("server listening on port 8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
