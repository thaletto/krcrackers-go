package server

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/thaletto/krcrackers-go/database"
	"github.com/thaletto/krcrackers-go/services/orders"
	"github.com/thaletto/krcrackers-go/services/products"
)

func NewHandler(db database.DB) http.Handler {
	mux := http.NewServeMux()
	humaConfig := huma.DefaultConfig("KR Crackers API", "1.0.0")
	humaConfig.OpenAPI.Info.Description = "KR Crackers API running on Cloudflare D1."
	api := humago.New(mux, humaConfig)

	productsSvc := products.NewService(db)
	ordersSvc := orders.NewService(db)

	productsSvc.RegisterRoutes(api)
	ordersSvc.RegisterRoutes(api)

	type healthResponse struct {
		Status  int    `json:"status" example:"200"`
		Message string `json:"message" example:"ok"`
	}

	type healthOutput struct {
		Body healthResponse
	}

	huma.Register(api, huma.Operation{
		OperationID: "health",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Health check",
		Tags:        []string{"system"},
	}, func(_ context.Context, _ *struct{}) (*healthOutput, error) {
		return &healthOutput{Body: healthResponse{Status: 200, Message: "ok"}}, nil
	})

	return withLogging(api.Adapter())
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
