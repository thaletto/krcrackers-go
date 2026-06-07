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

	"github.com/thaletto/krcrackers-go/config"
	"github.com/thaletto/krcrackers-go/database"
	"github.com/thaletto/krcrackers-go/migrations"
	"github.com/thaletto/krcrackers-go/services/orders"
	"github.com/thaletto/krcrackers-go/services/products"
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
	db, err := database.New(database.Config{
		Mode:  cfg.Backend,
		D1:    cfg.D1,
		Local: cfg.Local,
	})
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

	productsSvc := products.NewService(db)
	ordersSvc := orders.NewService(db)

	mux := http.NewServeMux()
	humaConfig := huma.DefaultConfig("KR Crackers API", "1.0.0")
	humaConfig.OpenAPI.Info.Description = "KR Crackers API running on Cloudflare D1."
	humaConfig.OpenAPI.Servers = []*huma.Server{
		{URL: "http://localhost:" + cfg.Port, Description: "Local"},
	}
	api := humago.New(mux, humaConfig)

	productsSvc.RegisterRoutes(api)
	ordersSvc.RegisterRoutes(api)

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

	log.Printf("starting server in %s mode on :%s (docs at /docs)", cfg.Backend, cfg.Port)
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
