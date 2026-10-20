package orders_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/thaletto/krcrackers-go/src/database"
	"github.com/thaletto/krcrackers-go/src/migrations"
	"github.com/thaletto/krcrackers-go/src/services/orders"
)

// newPricedFixture builds a Service whose catalogue is under the test's
// control, so the pricing rules can be exercised on their own.
func newPricedFixture(t *testing.T, catalogue orders.Catalogue) (*orders.Service, database.DB) {
	t.Helper()
	ctx := context.Background()

	db, err := database.New(database.Config{
		Mode:  database.ModeLocal,
		Local: &database.LocalConfig{Path: filepath.Join(t.TempDir(), "priced.sqlite")},
	})
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("migrations.Up: %v", err)
	}
	if _, err := db.Execute(ctx, `
		INSERT INTO products (name, description, price, category) VALUES ('Sparklers', 'Box of sparklers', 2500, 'Fireworks')
	`); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	if _, err := db.Execute(ctx, `
		INSERT INTO users (email, name, auth_provider, password_hash, role) VALUES ('shopper@example.com', 'User', 'email', '', 'customer')
	`); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	return orders.NewService(orders.NewRepository(db), orders.Intake{Catalogue: catalogue}, nil), db
}

// The bug this guards: the order took its prices and its total straight from
// the request body, so a client chose what it paid.
func TestClientCannotChooseThePrice(t *testing.T) {
	ctx := context.Background()
	catalogue := testCatalogue()
	svc, db := newPricedFixture(t, catalogue)

	input := orderInput("shopper@example.com")
	input.Total = 1
	input.Items = []orders.OrderItemFields{
		{ProductID: 1, ProductName: "Sparklers", Price: 1, Quantity: 4, Total: 4},
	}
	created, err := svc.Create(ctx, input, orders.Guest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if created.Total != 10000 {
		t.Errorf("order total = %v, want 10000 (catalogue price 2500 x 4)", created.Total)
	}
	if created.Items[0].Price != 2500 {
		t.Errorf("line price = %v, want the catalogue's 2500", created.Items[0].Price)
	}
	if created.Items[0].Total != 10000 {
		t.Errorf("line total = %v, want 10000", created.Items[0].Total)
	}
	if len(catalogue.asked) == 0 {
		t.Fatal("the catalogue was never consulted; the request price was used")
	}

	rows, err := db.Query(ctx, `SELECT total FROM orders WHERE id = ?`, created.ID)
	if err != nil {
		t.Fatalf("read order: %v", err)
	}
	stored, _ := rows[0].Float("total")
	if stored != 10000 {
		t.Errorf("stored total = %v, want 10000", stored)
	}
}

// The catalogue is the only source of product names too, so an order cannot
// be filed under a name the shop does not stock.
func TestClientCannotChooseTheProductName(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPricedFixture(t, testCatalogue())

	input := orderInput("shopper@example.com")
	input.Items = []orders.OrderItemFields{
		{ProductID: 1, ProductName: "Definitely Not What We Sell", Price: 2500, Quantity: 1, Total: 2500},
	}
	created, err := svc.Create(ctx, input, orders.Guest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Items[0].ProductName != "Sparklers" {
		t.Errorf("line name = %q, want the catalogue's Sparklers", created.Items[0].ProductName)
	}
}

// A basket line for a product the catalogue does not have is refused rather
// than stored at a price nobody set.
func TestBasketForAProductTheCatalogueDoesNotHave(t *testing.T) {
	ctx := context.Background()
	svc, db := newPricedFixture(t, testCatalogue())

	input := orderInput("shopper@example.com")
	input.Items = []orders.OrderItemFields{
		{ProductID: 999, ProductName: "Ghost", Price: 1, Quantity: 1, Total: 1},
	}
	if _, err := svc.Create(ctx, input, orders.Guest()); !errors.Is(err, orders.ErrUnknownProduct) {
		t.Fatalf("Create with an unknown product returned %v, want ErrUnknownProduct", err)
	}

	rows, err := db.Query(ctx, `SELECT COUNT(*) AS n FROM orders`)
	if err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if n, _ := rows[0].Int("n"); n != 0 {
		t.Errorf("%d orders were written for a product that does not exist", n)
	}
}

// A line asking for nothing is refused rather than stored at a zero total.
func TestBasketQuantityMustBePositive(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPricedFixture(t, testCatalogue())

	for _, qty := range []int{0, -1} {
		input := orderInput("shopper@example.com")
		input.Items = []orders.OrderItemFields{
			{ProductID: 1, ProductName: "Sparklers", Price: 2500, Quantity: qty, Total: 0},
		}
		if _, err := svc.Create(ctx, input, orders.Guest()); !errors.Is(err, orders.ErrInvalidQuantity) {
			t.Errorf("Create with quantity %d returned %v, want ErrInvalidQuantity", qty, err)
		}
	}
}

// Without a catalogue there is no price, so an order cannot be placed at all.
// The failure is named rather than being a panic further in.
func TestIntakeWithoutACatalogueIsNamed(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPricedFixture(t, nil)

	if _, err := svc.Create(ctx, orderInput("shopper@example.com"), orders.Guest()); !errors.Is(err, orders.ErrIntakeUnavailable) {
		t.Errorf("Create without a catalogue returned %v, want ErrIntakeUnavailable", err)
	}
}
