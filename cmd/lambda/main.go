package main

import (
	"log"
	"net/http"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/thaletto/krcrackers-go/config"
	"github.com/thaletto/krcrackers-go/database"
	"github.com/thaletto/krcrackers-go/server"
	"github.com/thaletto/krcrackers-go/services/orders"
	"github.com/thaletto/krcrackers-go/services/products"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := database.New(database.Config{
		Mode:  cfg.Backend,
		D1:    cfg.D1,
		Local: cfg.Local,
	})
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	handler := newHandler(db)
	adapter := httpadapter.NewV2(handler)

	lambda.Start(adapter.ProxyWithContext)
}

func newHandler(db database.DB) http.Handler {
	mux := http.NewServeMux()

	productsSvc := products.NewService(products.NewRepository(db))
	ordersSvc := orders.NewService(orders.NewRepository(db))

	productsSvc.RegisterRoutes(mux)
	ordersSvc.RegisterRoutes(mux)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		server.WriteJSON(w, http.StatusOK, map[string]any{
			"status":  200,
			"message": "ok",
		})
	})

	return server.WithLogging(mux)
}
