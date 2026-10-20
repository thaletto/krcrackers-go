package orders_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ordersapi "github.com/thaletto/krcrackers-go/src/apis/orders"
	apperrors "github.com/thaletto/krcrackers-go/src/errors"
	"github.com/thaletto/krcrackers-go/src/services/auth"
	svc "github.com/thaletto/krcrackers-go/src/services/orders"
)

// stubOrderBook stands in for the order module. The contract under test is
// the route table and the middleware in front of it, so the stub only has to
// answer; it is never asked to reproduce the module's behaviour.
type stubOrderBook struct {
	// calls records the Actor each method was reached with, so a test can
	// assert that a rejected request never reached the module at all.
	calls []string
	// lastFilter records the filter the most recent List was called with, so a
	// test can assert what the handler parsed out of the query string.
	lastFilter svc.ListFilter
}

var _ ordersapi.OrderBook = (*stubOrderBook)(nil)

func (s *stubOrderBook) record(name string) { s.calls = append(s.calls, name) }

func (s *stubOrderBook) Create(context.Context, svc.OrderInput, svc.Actor) (svc.Order, error) {
	s.record("Create")
	return svc.Order{ID: 1, OrderFields: svc.OrderFields{Status: svc.StatusPending}}, nil
}

func (s *stubOrderBook) List(_ context.Context, _ svc.Actor, filter svc.ListFilter) (svc.ListOrdersResponse, error) {
	s.record("List")
	s.lastFilter = filter
	return svc.ListOrdersResponse{}, nil
}

func (s *stubOrderBook) Get(_ context.Context, actor svc.Actor, id int) (svc.Order, error) {
	s.record("Get")
	if actor.IsGuest() {
		return svc.Order{}, apperrors.ErrNotFound
	}
	return svc.Order{ID: id}, nil
}

func (s *stubOrderBook) Update(_ context.Context, actor svc.Actor, id int, _ svc.OrderInput) (svc.Order, error) {
	s.record("Update")
	if actor.IsGuest() {
		return svc.Order{}, apperrors.ErrNotFound
	}
	return svc.Order{ID: id}, nil
}

func (s *stubOrderBook) Delete(_ context.Context, actor svc.Actor, _ int) error {
	s.record("Delete")
	if actor.IsGuest() {
		return apperrors.ErrNotFound
	}
	return nil
}

func (s *stubOrderBook) Checkout(context.Context, svc.Actor, int, []svc.CheckoutItem, io.Reader, string, string, string) (svc.Order, string, error) {
	s.record("Checkout")
	return svc.Order{}, "", apperrors.ErrNotFound
}

func (s *stubOrderBook) Cancel(_ context.Context, actor svc.Actor, id int) (svc.Order, error) {
	s.record("Cancel")
	if actor.IsGuest() {
		return svc.Order{}, apperrors.ErrNotFound
	}
	return svc.Order{ID: id}, nil
}

func (s *stubOrderBook) UpdateStatus(_ context.Context, actor svc.Actor, id int, _ svc.OrderStatus) (svc.Order, error) {
	s.record("UpdateStatus")
	if !actor.CanAdminister() {
		return svc.Order{}, svc.ErrNotPermitted
	}
	return svc.Order{ID: id}, nil
}

func (s *stubOrderBook) DashboardStats(_ context.Context, actor svc.Actor) (svc.DashboardStats, error) {
	s.record("DashboardStats")
	if !actor.CanAdminister() {
		return svc.DashboardStats{}, svc.ErrNotPermitted
	}
	return svc.DashboardStats{TotalOrders: 1}, nil
}

// newTestMux registers the real order routes over a stub module, with a JWT
// secret in place so the middleware can validate a token.
func newTestMux(t *testing.T, book ordersapi.OrderBook) *http.ServeMux {
	t.Helper()
	auth.SetJWTSecret("test-secret-for-route-checks")
	mux := http.NewServeMux()
	ordersapi.NewHandler(book, nil).RegisterRoutes(mux)
	return mux
}

func tokenFor(t *testing.T, userID int, role string) string {
	t.Helper()
	tok, err := auth.GenerateAccessToken(userID, role)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	return tok
}

func do(mux *http.ServeMux, method, path, token string) *httptest.ResponseRecorder {
	return doWithBody(mux, method, path, token, "")
}

func doWithBody(mux *http.ServeMux, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// The bug this guards: GET /orders, GET|PUT|DELETE /orders/{id} were
// registered with no middleware, so an unauthenticated caller could list, read,
// rewrite and delete every order in the book. The route table is the only place
// that mistake can be made, so the route table is what is asserted.
//
// It fails on the pre-fix code: those four requests returned 200 and reached
// the module instead of 401.
func TestOrderRoutesRejectAnUnauthenticatedCaller(t *testing.T) {
	book := &stubOrderBook{}
	mux := newTestMux(t, book)

	cases := []struct{ method, path string }{
		{http.MethodGet, "/orders"},
		{http.MethodGet, "/orders/1"},
		{http.MethodPut, "/orders/1"},
		{http.MethodDelete, "/orders/1"},
		{http.MethodGet, "/orders/my"},
		{http.MethodGet, "/orders/my/1"},
		{http.MethodDelete, "/orders/my/1"},
		{http.MethodPost, "/orders/checkout"},
		{http.MethodGet, "/admin/orders"},
		{http.MethodGet, "/admin/orders/1"},
		{http.MethodPatch, "/admin/orders/1/status"},
		{http.MethodGet, "/admin/dashboard"},
	}

	for _, c := range cases {
		rec := do(mux, c.method, c.path, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a session = %d, want 401", c.method, c.path, rec.Code)
		}
	}
	if len(book.calls) != 0 {
		t.Errorf("an unauthenticated request reached the order module: %v", book.calls)
	}
}

// A signed-in shopper may read and change their own orders, and an
// administrator may do the same to any.
func TestOrderRoutesAcceptTheRightRole(t *testing.T) {
	book := &stubOrderBook{}
	mux := newTestMux(t, book)
	shopper := tokenFor(t, 7, "customer")
	admin := tokenFor(t, 9, "admin")

	for _, c := range []struct{ method, path, token, body string }{
		{http.MethodGet, "/orders", shopper, ""},
		{http.MethodGet, "/orders/1", shopper, ""},
		{http.MethodGet, "/orders/my", shopper, ""},
		{http.MethodGet, "/admin/orders", admin, ""},
		{http.MethodPatch, "/admin/orders/1/status", admin, `{"status":"confirmed"}`},
		{http.MethodGet, "/admin/dashboard", admin, ""},
	} {
		rec := doWithBody(mux, c.method, c.path, c.token, c.body)
		if rec.Code >= 400 {
			t.Errorf("%s %s with the right role = %d, want success", c.method, c.path, rec.Code)
		}
	}
}

// Administrator routes refuse a signed-in shopper, and the refusal is 403 from
// the role check rather than an accident of ordering.
func TestAdminOrderRoutesRefuseAShopper(t *testing.T) {
	book := &stubOrderBook{}
	mux := newTestMux(t, book)
	shopper := tokenFor(t, 7, "customer")

	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/admin/orders"},
		{http.MethodGet, "/admin/orders/1"},
		{http.MethodPatch, "/admin/orders/1/status"},
		{http.MethodGet, "/admin/dashboard"},
	} {
		rec := do(mux, c.method, c.path, shopper)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as a shopper = %d, want 403", c.method, c.path, rec.Code)
		}
	}
}

// Creating an order stays open to guests: it is the storefront's checkout, and a
// signed-in shopper's order is simply filed under their account.
func TestCreatingAnOrderStaysOpenToGuests(t *testing.T) {
	book := &stubOrderBook{}
	mux := newTestMux(t, book)

	req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(`{
		"userName":"Walkup","email":"walkup@example.com","phone":"1",
		"street":"9 Guest St","townOrCity":"Mumbai","state":"MH","pincode":"400001",
		"deliveryRegion":"W","deliveryLocation":"Mumbai","total":1,
		"items":[{"productId":1,"productName":"P","price":1,"quantity":1,"total":1}]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("POST /orders as a guest = %d, want 201; the point is that it was not 401", rec.Code)
	}
	if len(book.calls) != 1 || book.calls[0] != "Create" {
		t.Errorf("module calls = %v, want [Create]; a guest must reach the module", book.calls)
	}
}

// The dashboard composes the Account module's figure, so a handler wired
// without one still renders the order numbers.
func TestDashboardRendersWithoutTheAccountFigure(t *testing.T) {
	book := &stubOrderBook{}
	mux := newTestMux(t, book)

	rec := do(mux, http.MethodGet, "/admin/dashboard", tokenFor(t, 9, "admin"))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/dashboard = %d, want 200", rec.Code)
	}

	var body struct {
		TotalOrders  int `json:"totalOrders"`
		NewCustomers int `json:"newCustomers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode dashboard: %v", err)
	}
	if body.TotalOrders != 1 {
		t.Errorf("totalOrders = %d, want the module's figure", body.TotalOrders)
	}
	if body.NewCustomers != 0 {
		t.Errorf("newCustomers = %d, want 0 when no account source is wired", body.NewCustomers)
	}
}
