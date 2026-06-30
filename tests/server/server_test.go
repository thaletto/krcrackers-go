package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thaletto/krcrackers-go/src/server"
)

func TestWriteJSONSetsContentTypeAndStatus(t *testing.T) {
	rr := httptest.NewRecorder()
	server.WriteJSON(rr, http.StatusTeapot, map[string]any{"hello": "world"})

	if got, want := rr.Header().Get("Content-Type"), "application/json"; got != want {
		t.Errorf("Content-Type: got %q, want %q", got, want)
	}
	if rr.Code != http.StatusTeapot {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusTeapot)
	}

	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["hello"] != "world" {
		t.Errorf("body: got %+v, want {hello:world}", body)
	}
}

func TestWriteErrorEmitsErrorResponse(t *testing.T) {
	rr := httptest.NewRecorder()
	server.WriteError(rr, http.StatusBadRequest, "missing field")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", rr.Code)
	}

	var body server.ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "missing field" {
		t.Errorf("error message: got %q, want %q", body.Error, "missing field")
	}
}

func TestStatusResponseShape(t *testing.T) {
	rr := httptest.NewRecorder()
	server.WriteJSON(rr, http.StatusOK, server.StatusResponse{Status: "ok"})

	var body server.StatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status: got %q, want %q", body.Status, "ok")
	}
}

func TestWithLoggingPassesThroughAndRecordsStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("GET /boom", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	})

	for _, tc := range []struct {
		path string
		want int
	}{
		{"/ok", http.StatusAccepted},
		{"/boom", http.StatusInternalServerError},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rr := httptest.NewRecorder()
		server.WithLogging(mux).ServeHTTP(rr, req)

		if rr.Code != tc.want {
			t.Errorf("%s: got %d, want %d", tc.path, rr.Code, tc.want)
		}
	}
}

func TestWriteJSONEncodesArbitraryStructs(t *testing.T) {
	type payload struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	rr := httptest.NewRecorder()
	server.WriteJSON(rr, http.StatusOK, payload{Name: "abc", Count: 7})

	body := rr.Body.String()
	if !strings.Contains(body, `"name":"abc"`) || !strings.Contains(body, `"count":7`) {
		t.Errorf("body missing fields: %q", body)
	}
}
