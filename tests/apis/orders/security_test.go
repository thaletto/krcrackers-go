package orders_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ordersapi "github.com/thaletto/krcrackers-go/src/apis/orders"
	ordersvc "github.com/thaletto/krcrackers-go/src/services/orders"
)

const orderBody = `{
	"userName":"Walkup","email":"walkup@example.com","phone":"1",
	"street":"9 Guest St","townOrCity":"Mumbai","state":"MH","pincode":"400001",
	"deliveryRegion":"W","deliveryLocation":"Mumbai","total":1,
	"items":[{"productId":1,"productName":"P","price":1,"quantity":1,"total":1}]
}`

func postOrder(mux *http.ServeMux, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// internalLeak stands in for a driver or transport error whose text is not the
// module's to publish.
type internalLeak struct{ detail string }

func (e *internalLeak) Error() string { return e.detail }

// leakyOrderBook returns a canned failure from the two methods a client can
// reach without a session, so the handler's error mapping is what is observed.
type leakyOrderBook struct {
	stubOrderBook
	failure error
}

func (l *leakyOrderBook) Create(context.Context, ordersvc.OrderInput, ordersvc.Actor) (ordersvc.Order, error) {
	return ordersvc.Order{}, l.failure
}

func (l *leakyOrderBook) Update(context.Context, ordersvc.Actor, int, ordersvc.OrderInput) (ordersvc.Order, error) {
	return ordersvc.Order{}, l.failure
}

var _ ordersapi.OrderBook = (*leakyOrderBook)(nil)

// The bug this guards: an internal error was reflected to the client verbatim.
// The chain from the driver to the response body is unbroken, and on D1 the
// terminal error is cloudflare-go's, whose text carries the request URL holding
// the Cloudflare account id and the D1 database id, plus the raw API error with
// the SQL. POST /orders is reachable without a session, so all of it was
// disclosed to anyone who asked.
//
// It fails on the pre-fix code: the response body contains the account id.
func TestInternalErrorsAreNotReflectedToTheClient(t *testing.T) {
	mux := newTestMux(t, &leakyOrderBook{failure: &internalLeak{
		`POST "https://api.cloudflare.com/client/v4/accounts/acc-12345/d1/database/db-67890/query": 400 ` +
			`{"success":false,"errors":[{"code":7400,"message":"D1_ERROR: no such table: users at line 1 column 27"}]}`,
	}})

	rec := postOrder(mux, orderBody)
	body := rec.Body.String()
	for _, secret := range []string{"acc-12345", "db-67890", "api.cloudflare.com", "D1_ERROR", "no such table"} {
		if strings.Contains(body, secret) {
			t.Errorf("response body leaked %q: %s", secret, body)
		}
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

// An error the client is entitled to know is still reported. The distinction is
// the sentinel, not the wording.
func TestClientFacingErrorsAreStillReported(t *testing.T) {
	mux := newTestMux(t, &leakyOrderBook{failure: ordersvc.ErrUnknownProduct})

	rec := postOrder(mux, orderBody)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "product not found") {
		t.Errorf("body = %s, want the sentinel's own message", rec.Body.String())
	}
}

// The bug this guards: `limit` went from the query string straight into the SQL,
// and a limit of 0 means "omitted" — so ?limit=1000000000 asked for the whole
// table, which the order list then hydrates with one query per order.
func TestPageLimitIsClamped(t *testing.T) {
	book := &stubOrderBook{}
	mux := newTestMux(t, book)
	token := tokenFor(t, 7, "customer")

	rec := doWithBody(mux, http.MethodGet, "/orders?limit=1000000000", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if book.lastFilter.Limit == 0 {
		t.Error("limit reached the module as 0, which means 'every order'")
	}
	if book.lastFilter.Limit > 100 {
		t.Errorf("limit = %d, want it clamped to 100", book.lastFilter.Limit)
	}

	// A malformed or negative value is treated as absent, not passed through.
	for _, q := range []string{"?limit=abc", "?limit=-5", "?offset=-1&limit=10"} {
		book.lastFilter = ordersvc.ListFilter{}
		if rec := doWithBody(mux, http.MethodGet, "/orders"+q, token, ""); rec.Code != http.StatusOK {
			t.Errorf("GET /orders%s = %d, want 200", q, rec.Code)
		}
		if book.lastFilter.Limit > 100 {
			t.Errorf("GET /orders%s reached the module with limit %d", q, book.lastFilter.Limit)
		}
		if book.lastFilter.Offset < 0 {
			t.Errorf("GET /orders%s reached the module with offset %d", q, book.lastFilter.Offset)
		}
	}
}
