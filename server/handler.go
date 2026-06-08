package server

import (
	"log"
	"net/http"
	"time"

	"github.com/thaletto/krcrackers-go/database"
	"github.com/thaletto/krcrackers-go/serverutil"
	"github.com/thaletto/krcrackers-go/services/orders"
	"github.com/thaletto/krcrackers-go/services/products"
)

func NewHandler(db database.DB) http.Handler {
	mux := http.NewServeMux()

	productsRepo := products.NewRepository(db)
	ordersRepo := orders.NewRepository(db)

	productsSvc := products.NewService(productsRepo)
	ordersSvc := orders.NewService(ordersRepo)

	productsSvc.RegisterRoutes(mux)
	ordersSvc.RegisterRoutes(mux)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		serverutil.WriteJSON(w, http.StatusOK, map[string]any{
			"status":  200,
			"message": "ok",
		})
	})

	return withLogging(mux)
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
