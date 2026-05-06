package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/example/monorepo/internal/api"
)

func main() {
	repo := &inMemoryRepo{}
	orderHandler := api.NewHandler(repo)
	paymentHandler := &api.PaymentHandler{Repo: repo}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders", orderHandler.CreateOrderHandler)
	mux.HandleFunc("POST /orders/{id}/cancel", orderHandler.CancelOrderHandler)
	mux.HandleFunc("GET /orders", orderHandler.ListOrdersHandler)
	mux.HandleFunc("POST /orders/{id}/pay", paymentHandler.ProcessHandler)

	addr := ":8080"
	fmt.Println("server listening on", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
