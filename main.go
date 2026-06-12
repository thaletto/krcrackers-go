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

	"github.com/thaletto/krcrackers-go/adapters"
	"github.com/thaletto/krcrackers-go/config"
	"github.com/thaletto/krcrackers-go/database"
	"github.com/thaletto/krcrackers-go/eventbus"
	"github.com/thaletto/krcrackers-go/migrations"
	"github.com/thaletto/krcrackers-go/server"
	"github.com/thaletto/krcrackers-go/services/auth"
	"github.com/thaletto/krcrackers-go/services/customers"
	"github.com/thaletto/krcrackers-go/services/invoices"
	"github.com/thaletto/krcrackers-go/services/notifications"
	"github.com/thaletto/krcrackers-go/services/orders"
	"github.com/thaletto/krcrackers-go/services/products"
	"github.com/thaletto/krcrackers-go/services/search"
	"github.com/thaletto/krcrackers-go/services/uploads"
)

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

	if cfg.Meilisearch.URL != "" && cfg.Meilisearch.APIKey != "" {
		s, err := search.NewService(cfg.Meilisearch.URL, cfg.Meilisearch.APIKey)
		if err != nil {
			log.Printf("warning: meilisearch unavailable: %v", err)
		} else {
			searchSub := search.NewSubscriber(s)
			searchSub.RegisterHandlers(bus)
		}
	}

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
