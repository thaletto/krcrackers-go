package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"
)

// ErrorResponse is the standard error response body.
type ErrorResponse struct {
	Error string `json:"error"`
}

// StatusResponse is a simple status response body.
type StatusResponse struct {
	Status string `json:"status"`
}

// MaxBodyBytes caps JSON request bodies to prevent oversized allocations.
const MaxBodyBytes = 1 << 20

// MaxUploadBytes caps multipart checkout bodies (form + screenshot).
const MaxUploadBytes = 10 << 20

// MaxPageLimit caps the `limit` a client may ask a list endpoint for.
//
// A limit of 0 means "omitted" and returns everything, so without a ceiling
// `?limit=1000000000` is a request for the whole table — and the order list
// hydrates line items with one query per order on top of it.
const MaxPageLimit = 100

// ParsePage reads the `limit` and `offset` query parameters, clamping limit to
// MaxPageLimit. A malformed or negative value is treated as absent rather than
// passed through to SQL.
func ParsePage(r *http.Request) (limit, offset int) {
	q := r.URL.Query()
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
		limit = min(v, MaxPageLimit)
	}
	if v, err := strconv.Atoi(q.Get("offset")); err == nil && v > 0 {
		offset = v
	}
	return limit, offset
}

// WriteInternalError reports a server-side failure without disclosing it.
//
// The real error goes to the log; the client gets a fixed message. An error
// string that crossed the seam intact has been observed carrying a Cloudflare
// account and database id, an R2 bucket name and a SQL fragment — all of them
// reachable from an unauthenticated request. Anything a client is entitled to
// know must be a sentinel it can match with errors.Is and raise as its own
// status code.
func WriteInternalError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	WriteError(w, http.StatusInternalServerError, "internal error")
}

// DecodeJSON decodes a capped JSON request body into v.
func DecodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	return json.NewDecoder(r.Body).Decode(v)
}

// DecodeJSONStrict decodes a capped JSON request body into v and rejects
// unknown fields, so a client that misspells a field is told rather than
// silently having the value dropped.
func DecodeJSONStrict(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, ErrorResponse{Error: msg})
}

// WithSecurityHeaders sets the response headers that cost nothing and close off
// the easy abuses: MIME sniffing, framing, and a plaintext first hop.
func WithSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func WithLogging(next http.Handler) http.Handler {
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
