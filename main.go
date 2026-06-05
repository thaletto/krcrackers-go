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

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"app/config"
	"app/database"
	"app/services/products"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := database.New(database.Config{
		Mode:       cfg.AppEnv,
		APIToken:   cfg.APIToken,
		AccountID:  cfg.AccountID,
		DatabaseID: cfg.DatabaseID,
		LocalPath:  cfg.LocalPath,
	})
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	migrateCtx, cancelMigrate := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelMigrate()

	productsSvc := products.NewService(db)
	if err := productsSvc.Migrate(migrateCtx); err != nil {
		log.Fatalf("migrate products: %v", err)
	}

	mux := http.NewServeMux()
	humaConfig := huma.DefaultConfig("KR Crackers API", "1.0.0")
	humaConfig.OpenAPI.Info.Description = "KR Crackers API running on Cloudflare D1."
	humaConfig.OpenAPI.Servers = []*huma.Server{
		{URL: "http://localhost:" + cfg.Port, Description: "Local"},
	}
	api := humago.New(mux, humaConfig)

	productsSvc.RegisterRoutes(api)

	huma.Register(api, huma.Operation{
		OperationID: "health",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Health check",
		Tags:        []string{"system"},
	}, func(_ context.Context, _ *struct{}) (*struct{}, error) {
		return &struct{}{}, nil
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           withLogging(api.Adapter()),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("starting server in %s mode on :%s (docs at /docs)", cfg.AppEnv, cfg.Port)
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

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rw.status, time.Since(start))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
