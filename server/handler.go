package server

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/thaletto/krcrackers-go/database"
	"github.com/thaletto/krcrackers-go/services/orders"
	"github.com/thaletto/krcrackers-go/services/products"
)

func NewHandler(db database.DB) http.Handler {
	mux := http.NewServeMux()

	productsSvc := products.NewService(db)
	ordersSvc := orders.NewService(db)

	productsSvc.RegisterRoutes(mux)
	ordersSvc.RegisterRoutes(mux)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  200,
			"message": "ok",
		})
	})

	return withLogging(mux)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
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
