package orders_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/thaletto/krcrackers-go/src/database"
	"github.com/thaletto/krcrackers-go/src/migrations"
	"github.com/thaletto/krcrackers-go/src/services/orders"
)

func newOrderService(t *testing.T) (*orders.Service, database.DB) {
	t.Helper()

	db, err := database.New(database.Config{
		Mode:  database.ModeLocal,
		Local: &database.LocalConfig{Path: filepath.Join(t.TempDir(), "orders.sqlite")},
	})
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := migrations.Up(context.Background(), db); err != nil {
		t.Fatalf("migrations.Up: %v", err)
	}

	// orders reference a product, and a linked order references an account.
	// Both are enforced as foreign keys, so the fixtures have to exist.
	if _, err := db.Execute(context.Background(), `
		INSERT INTO products (name, description, price, category)
		VALUES ('Sparklers', 'Box of sparklers', 2500, 'Fireworks')
	`); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	for _, email := range []string{"shopper@example.com", "other@example.com", "attacker@example.com"} {
		if _, err := db.Execute(context.Background(), `
			INSERT INTO users (email, name, auth_provider, password_hash, role)
			VALUES (?, 'User', 'email', '', 'customer')
		`, email); err != nil {
			t.Fatalf("insert user %s: %v", email, err)
		}
	}

	return orders.NewService(orders.NewRepository(db), nil, nil, nil, nil), db
}

func orderInput(email string) orders.OrderInput {
	return orders.OrderInput{
		OrderFields: orders.OrderFields{
			UserName:         "Test User",
			Email:            email,
			Phone:            "9999999999",
			Street:           "1 Test Street",
			TownOrCity:       "Mumbai",
			State:            "Maharashtra",
			Pincode:          "400001",
			DeliveryRegion:   "West",
			DeliveryLocation: "Mumbai",
			Total:            2500,
		},
		Items: []orders.OrderItemFields{
			{ProductID: 1, ProductName: "Sparklers", Price: 2500, Quantity: 1, Total: 2500},
		},
	}
}

// A guest order carries no owner, so it must not show up in anybody's history.
func TestCreateWithoutOwnerLeavesOrderUnowned(t *testing.T) {
	ctx := context.Background()
	svc, _ := newOrderService(t)

	created, err := svc.Create(ctx, orderInput("guest@example.com"), 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.UserID != nil {
		t.Errorf("guest order got userId %d, want none", *created.UserID)
	}

	list, err := svc.ListForUser(ctx, 1, 0, 0)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("guest order leaked into a customer's history: %d items", len(list.Items))
	}
}

// userID looks up a seeded account by email. Ownership is a foreign key, so a
// test cannot invent an id.
func userID(t *testing.T, db database.DB, email string) int {
	t.Helper()
	rows, err := db.Query(context.Background(), `SELECT id FROM users WHERE email = ?`, email)
	if err != nil {
		t.Fatalf("query user %s: %v", email, err)
	}
	if len(rows) == 0 {
		t.Fatalf("fixture user %s was not seeded", email)
	}
	id, err := rows[0].Int("id")
	if err != nil {
		t.Fatalf("read user id: %v", err)
	}
	return int(id)
}

// The bug this guards: an order placed by a signed-in shopper was written with
// a NULL user_id, so it appeared in the admin dashboard (unfiltered) and
// nowhere in the shopper's own order history.
func TestCreateForSignedInShopperAppearsInTheirHistory(t *testing.T) {
	ctx := context.Background()
	svc, db := newOrderService(t)

	shopperID := userID(t, db, "shopper@example.com")
	strangerID := userID(t, db, "other@example.com")

	if _, err := svc.Create(ctx, orderInput("shopper@example.com"), shopperID); err != nil {
		t.Fatalf("Create: %v", err)
	}

	mine, err := svc.ListForUser(ctx, shopperID, 0, 0)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(mine.Items) != 1 {
		t.Fatalf("shopper's own history has %d orders, want 1", len(mine.Items))
	}
	if mine.Items[0].UserID == nil || *mine.Items[0].UserID != shopperID {
		t.Errorf("listed order userId = %v, want %d", mine.Items[0].UserID, shopperID)
	}

	// The order is still reachable by its owner and by nobody else.
	if _, err := svc.GetForUser(ctx, mine.Items[0].ID, shopperID); err != nil {
		t.Errorf("GetForUser(owner): %v", err)
	}
	if _, err := svc.GetForUser(ctx, mine.Items[0].ID, strangerID); err == nil {
		t.Error("another account could read the order; user_id is not the access check")
	}

	others, err := svc.ListForUser(ctx, strangerID, 0, 0)
	if err != nil {
		t.Fatalf("ListForUser(other): %v", err)
	}
	if len(others.Items) != 0 {
		t.Errorf("another account sees %d of somebody else's orders", len(others.Items))
	}
}

// Ownership comes from the session, never the payload: a client that knows a
// stranger's id must not be able to file an order under their account, which
// would hand that stranger read and cancel access to the order.
func TestCreateIgnoresOwnerFromPayload(t *testing.T) {
	ctx := context.Background()
	svc, db := newOrderService(t)

	input := orderInput("attacker@example.com")
	spoofed := userID(t, db, "other@example.com")
	input.UserID = &spoofed

	created, err := svc.Create(ctx, input, 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.UserID != nil {
		t.Errorf("order was filed under userId %d taken from the body, want none", *created.UserID)
	}

	stolen, err := svc.ListForUser(ctx, spoofed, 0, 0)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(stolen.Items) != 0 {
		t.Errorf("spoofed order landed in user %d's history", spoofed)
	}
}
