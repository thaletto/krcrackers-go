package orders_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thaletto/krcrackers-go/src/database"
	apperrors "github.com/thaletto/krcrackers-go/src/errors"
	"github.com/thaletto/krcrackers-go/src/migrations"
	"github.com/thaletto/krcrackers-go/src/services/orders"
)

// fakeShoppers and fakeAddresses stand in for the modules that own the users
// and customer_addresses tables. This is the second adapter at each seam: the
// production ones go through those modules, these need no database, so the
// intake rules can be exercised on their own.
type fakeShoppers struct{ byID map[int]orders.User }

func (f *fakeShoppers) GetUser(_ context.Context, id int) (orders.User, error) {
	return f.byID[id], nil
}

// fakeAddresses scopes its lookups the way the Customer module's query does: an
// address belonging to somebody else misses here, rather than coming back for
// the Order module to reject.
type fakeAddresses struct{ byID map[int]orders.Address }

func (f *fakeAddresses) AddressFor(_ context.Context, shopperID, addressID int) (orders.Address, error) {
	addr, ok := f.byID[addressID]
	if !ok || addr.UserID != shopperID {
		return orders.Address{}, orders.ErrAddressNotFound
	}
	return addr, nil
}

// checkoutFixture is a migrated database with one product, two accounts, and
// one saved address each.
type checkoutFixture struct {
	svc         *orders.Service
	db          database.DB
	shopperID   int
	strangerID  int
	ownAddrID   int
	foreignAddr int
}

func newCheckoutFixture(t *testing.T) checkoutFixture {
	t.Helper()
	ctx := context.Background()

	db, err := database.New(database.Config{
		Mode:  database.ModeLocal,
		Local: &database.LocalConfig{Path: filepath.Join(t.TempDir(), "checkout.sqlite")},
	})
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("migrations.Up: %v", err)
	}
	if _, err := db.Execute(ctx, `
		INSERT INTO products (name, description, price, category)
		VALUES ('Sparklers', 'Box of sparklers', 2500, 'Fireworks')
	`); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	for _, email := range []string{"shopper@example.com", "other@example.com"} {
		if _, err := db.Execute(ctx, `
			INSERT INTO users (email, name, auth_provider, password_hash, role)
			VALUES (?, 'User', 'email', '', 'customer')
		`, email); err != nil {
			t.Fatalf("insert user %s: %v", email, err)
		}
	}

	shopperID := userID(t, db, "shopper@example.com")
	strangerID := userID(t, db, "other@example.com")

	shoppers := &fakeShoppers{byID: map[int]orders.User{
		shopperID:  {ID: shopperID, Name: "Shopper", Email: "shopper@example.com", Phone: "9000000001"},
		strangerID: {ID: strangerID, Name: "Other", Email: "other@example.com", Phone: "9000000002"},
	}}
	addresses := &fakeAddresses{byID: map[int]orders.Address{
		1: {ID: 1, UserID: shopperID, Label: "Home", Street: "1 Test Street", City: "Mumbai", State: "Maharashtra", Pincode: "400001", Country: "India"},
		2: {ID: 2, UserID: strangerID, Label: "Work", Street: "2 Other Road", City: "Pune", State: "Maharashtra", Pincode: "411001", Country: "India"},
	}}

	return checkoutFixture{
		svc: orders.NewService(orders.NewRepository(db), orders.Intake{
			Shoppers:  shoppers,
			Addresses: addresses,
			Catalogue: testCatalogue(),
		}, nil),
		db:          db,
		shopperID:   shopperID,
		strangerID:  strangerID,
		ownAddrID:   1,
		foreignAddr: 2,
	}
}

func basket() []orders.CheckoutItem {
	return []orders.CheckoutItem{{ProductID: 1, ProductName: "Sparklers", Price: 2500, Quantity: 2}}
}

// The bug this guards: the address lookup was unscoped, so a shopper could
// name somebody else's saved address and have it copied onto their order.
func TestCheckoutRejectsAnAddressTheShopperDoesNotOwn(t *testing.T) {
	ctx := context.Background()
	f := newCheckoutFixture(t)

	_, _, err := f.svc.Checkout(ctx, orders.Shopper(f.shopperID), f.foreignAddr, basket(), nil, "", "", "")
	if !errors.Is(err, orders.ErrAddressNotFound) {
		t.Fatalf("checkout to a stranger's address returned %v, want ErrAddressNotFound", err)
	}

	rows, err := f.db.Query(ctx, `SELECT COUNT(*) AS n FROM orders`)
	if err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if n, _ := rows[0].Int("n"); n != 0 {
		t.Errorf("%d orders were written despite the rejected address", n)
	}
}

// The happy path still files the order under the shopper and snapshots their
// own address onto it.
func TestCheckoutFilesTheOrderUnderTheShopper(t *testing.T) {
	ctx := context.Background()
	f := newCheckoutFixture(t)

	order, _, err := f.svc.Checkout(ctx, orders.Shopper(f.shopperID), f.ownAddrID, basket(), nil, "", "", "UPI-123")
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if order.UserID == nil || *order.UserID != f.shopperID {
		t.Errorf("order userId = %v, want %d", order.UserID, f.shopperID)
	}
	if order.Street != "1 Test Street" {
		t.Errorf("order street = %q, want the shopper's own address", order.Street)
	}
	if order.Total != 5000 {
		t.Errorf("order total = %v, want 5000", order.Total)
	}
	if order.PaymentReference != "UPI-123" {
		t.Errorf("payment reference = %q; the payment fields were being dropped on insert", order.PaymentReference)
	}

	// The order is immediately readable by its owner and invisible to others.
	if _, err := f.svc.Get(ctx, orders.Shopper(f.shopperID), order.ID); err != nil {
		t.Errorf("owner cannot read their own order: %v", err)
	}
	if _, err := f.svc.Get(ctx, orders.Shopper(f.strangerID), order.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a stranger read the order (err = %v)", err)
	}
}

// Uploading a screenshot needs storage; without it the failure is named, not
// guessed at by comparing message text in the HTTP layer.
func TestCheckoutNamesTheMissingUploadConfiguration(t *testing.T) {
	ctx := context.Background()
	f := newCheckoutFixture(t)

	_, _, err := f.svc.Checkout(ctx, orders.Shopper(f.shopperID), f.ownAddrID, basket(),
		strings.NewReader("png"), "shot.png", "image/png", "")
	if !errors.Is(err, orders.ErrUploadsUnavailable) {
		t.Fatalf("Checkout with no uploads returned %v, want ErrUploadsUnavailable", err)
	}
}
