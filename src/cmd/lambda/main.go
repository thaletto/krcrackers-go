package main

import (
	"log"
	"net/http"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/thaletto/krcrackers-go/adapters"
	"github.com/thaletto/krcrackers-go/config"
	"github.com/thaletto/krcrackers-go/database"
	"github.com/thaletto/krcrackers-go/eventbus"
	"github.com/thaletto/krcrackers-go/server"
	"github.com/thaletto/krcrackers-go/services/auth"
	"github.com/thaletto/krcrackers-go/services/customers"
	"github.com/thaletto/krcrackers-go/services/invoices"
	"github.com/thaletto/krcrackers-go/services/notifications"
	"github.com/thaletto/krcrackers-go/services/orders"
	"github.com/thaletto/krcrackers-go/services/products"
	"github.com/thaletto/krcrackers-go/services/uploads"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	handler := newHandler(db, cfg)
	adapter := httpadapter.NewV2(handler)

	lambda.Start(adapter.ProxyWithContext)
}

func newHandler(db database.DB, cfg *config.Config) http.Handler {
	mux := http.NewServeMux()

	bus := eventbus.New()

	var uploadSvc uploads.Service
	if cfg.R2.AccessKeyID != "" && cfg.R2.SecretKey != "" && cfg.R2.BucketName != "" {
		s, err := uploads.NewService(cfg.R2.AccountID, cfg.R2.AccessKeyID, cfg.R2.SecretKey, cfg.R2.BucketName, cfg.R2.PublicURLBase)
		if err != nil {
			log.Printf("warning: r2 uploads unavailable: %v", err)
		} else {
			uploadSvc = s
		}
	}

	authRepo := auth.NewRepository(db)
	authSvc := auth.NewService(authRepo, cfg.JWT.Secret, cfg.IsProduction)

	customersRepo := customers.NewRepository(db)
	customersSvc := customers.NewService(customersRepo)

	notifSvc := notifications.NewWhatsAppService(cfg.WhatsApp.APIToken, cfg.WhatsApp.PhoneNumberID, cfg.WhatsApp.FromNumber)
	notifSub := notifications.NewSubscriber(notifSvc)
	notifSub.RegisterHandlers(bus)

	productsRepo := products.NewRepository(db)
	productsSvc := products.NewService(productsRepo, bus)

	userAdapter := &adapters.UserProviderAdapter{Repo: authRepo}
	addrAdapter := &adapters.AddressProviderAdapter{DB: db}
	ordersRepo := orders.NewRepository(db)
	ordersSvc := orders.NewService(ordersRepo, userAdapter, addrAdapter, uploadSvc, bus)

	invoicesRepo := invoices.NewRepository(db)
	invoicesSvc := invoices.NewService(invoicesRepo)

	authSvc.RegisterRoutes(mux)
	customersSvc.RegisterRoutes(mux)
	productsSvc.RegisterRoutes(mux)
	ordersSvc.RegisterRoutes(mux)
	invoicesSvc.RegisterRoutes(mux)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		server.WriteJSON(w, http.StatusOK, map[string]any{
			"status":  200,
			"message": "ok",
		})
	})

	return server.WithLogging(mux)
}
