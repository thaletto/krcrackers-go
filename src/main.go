package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thaletto/krcrackers-go/src/adapters"
	authapi "github.com/thaletto/krcrackers-go/src/apis/auth"
	customersapi "github.com/thaletto/krcrackers-go/src/apis/customers"
	invoicesapi "github.com/thaletto/krcrackers-go/src/apis/invoices"
	ordersapi "github.com/thaletto/krcrackers-go/src/apis/orders"
	"github.com/thaletto/krcrackers-go/src/apis/products"
	"github.com/thaletto/krcrackers-go/src/config"
	"github.com/thaletto/krcrackers-go/src/database"
	"github.com/thaletto/krcrackers-go/src/eventbus"
	"github.com/thaletto/krcrackers-go/src/migrations"
	"github.com/thaletto/krcrackers-go/src/server"
	"github.com/thaletto/krcrackers-go/src/services/auth"
	"github.com/thaletto/krcrackers-go/src/services/customers"
	"github.com/thaletto/krcrackers-go/src/services/invoices"
	"github.com/thaletto/krcrackers-go/src/services/notifications"
	"github.com/thaletto/krcrackers-go/src/services/orders"
	productsSvc "github.com/thaletto/krcrackers-go/src/services/products"
	"github.com/thaletto/krcrackers-go/src/services/uploads"
)

// @title           KR Crackers API
// @version         1.0
// @description     E-commerce backend for KR Crackers with order management, product catalog, and admin dashboard.
// @host            localhost:8080
// @BasePath        /
// @securityDefinitions.apikey cookieAuth
// @in cookie
// @name access_token
func main() {
	if len(os.Args) >= 2 && os.Args[1] == "migrate" {
		if err := runMigrate(os.Args[2:]); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		return
	}
	runServer()
}

func bootstrap() (database.DB, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}
	db, err := database.New(cfg.Database)
	if err != nil {
		return nil, nil, fmt.Errorf("database: %w", err)
	}
	return db, cfg, nil
}

func runMigrate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: migrate <up|down|status>")
	}
	db, _, err := bootstrap()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	switch args[0] {
	case "up":
		n, err := migrations.Up(ctx, db)
		if err != nil {
			return err
		}
		log.Printf("migrate: applied %d migration(s)", n)
	case "down":
		if err := migrations.Down(ctx, db); err != nil {
			return err
		}
	case "status":
		statuses, err := migrations.GetStatus(ctx, db)
		if err != nil {
			return err
		}
		for _, s := range statuses {
			fmt.Println(s)
		}
	default:
		return fmt.Errorf("unknown migrate subcommand %q (expected up, down, status)", args[0])
	}
	return nil
}

func runServer() {
	db, cfg, err := bootstrap()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	handler := newHandler(db, cfg)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("starting server in %s mode on :%s", cfg.Database.Mode, cfg.Port)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	fmt.Println("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
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
	authSvc := auth.NewService(authRepo, cfg.JWT.Secret, cfg.Google.ClientID)
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
