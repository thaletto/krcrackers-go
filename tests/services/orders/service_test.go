package orders_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/thaletto/krcrackers-go/src/database"
	apperrors "github.com/thaletto/krcrackers-go/src/errors"
	"github.com/thaletto/krcrackers-go/src/migrations"
	"github.com/thaletto/krcrackers-go/src/services/orders"
)

// fakeCatalogue is the second adapter at the Order module's Catalogue seam.
// The production one reads the Product module; this one answers from a map, so
// the pricing rules can be tested without a product table behind them.
type fakeCatalogue struct {
	prices map[int]orders.PricedProduct
	// asked records the id sets the Order module requested, so a test can
	// assert it went to the catalogue rather than trusting the request.
	asked [][]int
}

func (f *fakeCatalogue) Prices(_ context.Context, productIDs []int) (map[int]orders.PricedProduct, error) {
	f.asked = append(f.asked, productIDs)
	out := make(map[int]orders.PricedProduct, len(productIDs))
	for _, id := range productIDs {
		if p, ok := f.prices[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

// testCatalogue prices the single seeded product at 2500, the figure the
// products fixture inserts.
func testCatalogue() *fakeCatalogue {
	return &fakeCatalogue{prices: map[int]orders.PricedProduct{
		1: {Name: "Sparklers", Price: 2500},
	}}
}

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

	return orders.NewService(orders.NewRepository(db), orders.Intake{Catalogue: testCatalogue()}, nil), db
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

// A guest order carries no owner, so it must not show up in anybody's history.
func TestCreateWithoutOwnerLeavesOrderUnowned(t *testing.T) {
	ctx := context.Background()
	svc, db := newOrderService(t)
	shopper := userID(t, db, "shopper@example.com")

	created, err := svc.Create(ctx, orderInput("guest@example.com"), orders.Guest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.UserID != nil {
		t.Errorf("guest order got userId %d, want none", *created.UserID)
	}

	list, err := svc.List(ctx, orders.Shopper(shopper), orders.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("guest order leaked into a customer's history: %d items", len(list.Items))
	}
}

// The bug this guards: an order placed by a signed-in shopper was written with
// a NULL user_id, so it appeared in the admin dashboard (unfiltered) and
// nowhere in the shopper's own order history.
func TestCreateForSignedInShopperAppearsInTheirHistory(t *testing.T) {
	ctx := context.Background()
	svc, db := newOrderService(t)

	shopperID := userID(t, db, "shopper@example.com")
	strangerID := userID(t, db, "other@example.com")

	if _, err := svc.Create(ctx, orderInput("shopper@example.com"), orders.Shopper(shopperID)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	mine, err := svc.List(ctx, orders.Shopper(shopperID), orders.ListFilter{})
	if err != nil {
		t.Fatalf("List(owner): %v", err)
	}
	if len(mine.Items) != 1 {
		t.Fatalf("shopper's own history has %d orders, want 1", len(mine.Items))
	}
	if mine.Items[0].UserID == nil || *mine.Items[0].UserID != shopperID {
		t.Errorf("listed order userId = %v, want %d", mine.Items[0].UserID, shopperID)
	}

	// The order is still reachable by its owner and by nobody else.
	if _, err := svc.Get(ctx, orders.Shopper(shopperID), mine.Items[0].ID); err != nil {
		t.Errorf("Get(owner): %v", err)
	}
	if _, err := svc.Get(ctx, orders.Shopper(strangerID), mine.Items[0].ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("another account read the order (err = %v); user_id is not the access check", err)
	}

	others, err := svc.List(ctx, orders.Shopper(strangerID), orders.ListFilter{})
	if err != nil {
		t.Fatalf("List(other): %v", err)
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

	created, err := svc.Create(ctx, input, orders.Guest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.UserID != nil {
		t.Errorf("order was filed under userId %d taken from the body, want none", *created.UserID)
	}

	stolen, err := svc.List(ctx, orders.Shopper(spoofed), orders.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(stolen.Items) != 0 {
		t.Errorf("spoofed order landed in user %d's history", spoofed)
	}
}

// A guest has no account, so there is no id they could be handed that resolves
// to an order. Every guest read must miss.
func TestGuestCannotReadAnyOrder(t *testing.T) {
	ctx := context.Background()
	svc, db := newOrderService(t)
	shopperID := userID(t, db, "shopper@example.com")

	created, err := svc.Create(ctx, orderInput("shopper@example.com"), orders.Shopper(shopperID))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.Get(ctx, orders.Guest(), created.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("guest read order %d (err = %v), want ErrNotFound", created.ID, err)
	}
	list, err := svc.List(ctx, orders.Guest(), orders.ListFilter{})
	if err != nil {
		t.Fatalf("List(guest): %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("guest listed %d orders", len(list.Items))
	}
}

// An administrator reaches any order, on every operation that takes a Scope.
func TestAdministratorSeesEveryOrder(t *testing.T) {
	ctx := context.Background()
	svc, db := newOrderService(t)
	shopperID := userID(t, db, "shopper@example.com")
	adminID := userID(t, db, "other@example.com")

	mine, err := svc.Create(ctx, orderInput("shopper@example.com"), orders.Shopper(shopperID))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	guest, err := svc.Create(ctx, orderInput("guest@example.com"), orders.Guest())
	if err != nil {
		t.Fatalf("Create(guest): %v", err)
	}

	admin := orders.Administrator(adminID)
	for _, id := range []int{mine.ID, guest.ID} {
		if _, err := svc.Get(ctx, admin, id); err != nil {
			t.Errorf("admin Get(%d): %v", id, err)
		}
	}

	list, err := svc.List(ctx, admin, orders.ListFilter{})
	if err != nil {
		t.Fatalf("List(admin): %v", err)
	}
	if list.Total != 2 {
		t.Errorf("admin listed %d orders, want 2", list.Total)
	}
}

// The whole point of the actor-scoped write: a shopper who learns another
// shopper's order id must not be able to rewrite or delete it.
func TestShopperCannotMutateAnotherShoppersOrder(t *testing.T) {
	ctx := context.Background()
	svc, db := newOrderService(t)
	shopperID := userID(t, db, "shopper@example.com")
	strangerID := userID(t, db, "other@example.com")

	victim, err := svc.Create(ctx, orderInput("shopper@example.com"), orders.Shopper(shopperID))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	attacker := orders.Shopper(strangerID)

	hijacked := orderInput("attacker@example.com")
	if _, err := svc.Update(ctx, attacker, victim.ID, hijacked); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("Update on a stranger's order returned %v, want ErrNotFound", err)
	}
	if err := svc.Delete(ctx, attacker, victim.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("Delete on a stranger's order returned %v, want ErrNotFound", err)
	}

	survivor, err := svc.Get(ctx, orders.Shopper(shopperID), victim.ID)
	if err != nil {
		t.Fatalf("victim's order is gone: %v", err)
	}
	if survivor.Email != "shopper@example.com" {
		t.Errorf("order was rewritten to %q", survivor.Email)
	}
}

// The module refuses administrative work for a non-administrator, so a route
// registered without the admin middleware still cannot administer orders.
func TestNonAdministratorIsRefusedAdminOperations(t *testing.T) {
	ctx := context.Background()
	svc, db := newOrderService(t)
	shopperID := userID(t, db, "shopper@example.com")

	created, err := svc.Create(ctx, orderInput("shopper@example.com"), orders.Shopper(shopperID))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	shopper := orders.Shopper(shopperID)

	if _, err := svc.UpdateStatus(ctx, shopper, created.ID, orders.StatusConfirmed); !errors.Is(err, orders.ErrNotPermitted) {
		t.Errorf("UpdateStatus by a shopper returned %v, want ErrNotPermitted", err)
	}
	if _, err := svc.DashboardStats(ctx, shopper); !errors.Is(err, orders.ErrNotPermitted) {
		t.Errorf("DashboardStats by a shopper returned %v, want ErrNotPermitted", err)
	}
	if _, err := svc.Get(ctx, shopper, created.ID); err != nil {
		t.Errorf("the order changed state anyway: %v", err)
	}

	after, err := svc.Get(ctx, shopper, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if after.Status != orders.StatusPending {
		t.Errorf("status = %q, want pending", after.Status)
	}
}

// The invoice route reads an order through the Order module, so it inherits
// the same ownership check rather than deciding its own.
func TestInvoiceViewIsScopedToTheActor(t *testing.T) {
	ctx := context.Background()
	svc, db := newOrderService(t)
	shopperID := userID(t, db, "shopper@example.com")
	strangerID := userID(t, db, "other@example.com")

	created, err := svc.Create(ctx, orderInput("shopper@example.com"), orders.Shopper(shopperID))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	view, err := svc.InvoiceView(ctx, orders.Shopper(shopperID), created.ID)
	if err != nil {
		t.Fatalf("InvoiceView(owner): %v", err)
	}
	if view.Email != "shopper@example.com" {
		t.Errorf("invoice email = %q, want the owner's", view.Email)
	}
	if len(view.Items) != 1 {
		t.Errorf("invoice has %d lines, want 1", len(view.Items))
	}

	if _, err := svc.InvoiceView(ctx, orders.Shopper(strangerID), created.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a stranger obtained an invoice view (err = %v)", err)
	}
	if err := svc.AssignInvoiceNumber(ctx, orders.Shopper(strangerID), created.ID, "INV-0001"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a stranger wrote an invoice number (err = %v)", err)
	}
}
