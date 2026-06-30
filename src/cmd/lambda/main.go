package main

import (
	"log"
	"net/http"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/thaletto/krcrackers-go/src/adapters"
	authapi "github.com/thaletto/krcrackers-go/src/apis/auth"
	customersapi "github.com/thaletto/krcrackers-go/src/apis/customers"
	invoicesapi "github.com/thaletto/krcrackers-go/src/apis/invoices"
	ordersapi "github.com/thaletto/krcrackers-go/src/apis/orders"
	"github.com/thaletto/krcrackers-go/src/apis/products"
	"github.com/thaletto/krcrackers-go/src/config"
	"github.com/thaletto/krcrackers-go/src/database"
	"github.com/thaletto/krcrackers-go/src/eventbus"
	"github.com/thaletto/krcrackers-go/src/server"
	"github.com/thaletto/krcrackers-go/src/services/auth"
	"github.com/thaletto/krcrackers-go/src/services/customers"
	"github.com/thaletto/krcrackers-go/src/services/invoices"
	"github.com/thaletto/krcrackers-go/src/services/notifications"
	"github.com/thaletto/krcrackers-go/src/services/orders"
	productsSvc "github.com/thaletto/krcrackers-go/src/services/products"
	"github.com/thaletto/krcrackers-go/src/services/uploads"
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
	authSvc := auth.NewService(authRepo, cfg.JWT.Secret)
	authHandler := authapi.NewHandler(authSvc, cfg.IsProduction)

	customersRepo := customers.NewRepository(db)
	customersSvc := customers.NewService(customersRepo)
	customersHandler := customersapi.NewHandler(customersSvc)

	notifSvc := notifications.NewWhatsAppService(cfg.WhatsApp.APIToken, cfg.WhatsApp.PhoneNumberID, cfg.WhatsApp.FromNumber)
	notifSub := notifications.NewSubscriber(notifSvc)
	notifSub.RegisterHandlers(bus)

	productsSvc := productsSvc.NewService(db, bus)
	productsHandler := products.NewHandler(productsSvc)

	userAdapter := &adapters.UserProviderAdapter{Repo: authRepo}
	addrAdapter := &adapters.AddressProviderAdapter{DB: db}
	ordersRepo := orders.NewRepository(db)
	ordersSvc := orders.NewService(ordersRepo, userAdapter, addrAdapter, uploadSvc, bus)
	ordersHandler := ordersapi.NewHandler(ordersSvc)

	invoicesRepo := invoices.NewRepository(db)
	invoicesSvc := invoices.NewService(invoicesRepo)
	invoicesHandler := invoicesapi.NewHandler(invoicesSvc)

	authHandler.RegisterRoutes(mux)
	customersHandler.RegisterRoutes(mux)
	productsHandler.RegisterRoutes(mux)
	ordersHandler.RegisterRoutes(mux)
	invoicesHandler.RegisterRoutes(mux)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		server.WriteJSON(w, http.StatusOK, map[string]any{
			"status":  200,
			"message": "ok",
		})
	})

	return server.WithLogging(mux)
}
