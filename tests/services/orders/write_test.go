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

// newWriteFixture is a migrated database with one product, one customer and
// one administrator.
func newWriteFixture(t *testing.T) (*orders.Service, database.DB, int, int) {
	t.Helper()
	ctx := context.Background()

	db, err := database.New(database.Config{
		Mode:  database.ModeLocal,
		Local: &database.LocalConfig{Path: filepath.Join(t.TempDir(), "write.sqlite")},
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
	for _, email := range []string{"shopper@example.com", "other@example.com"} {
		if _, err := db.Execute(ctx, `
			INSERT INTO users (email, name, auth_provider, password_hash, role) VALUES (?, 'User', 'email', '', 'customer')
		`, email); err != nil {
			t.Fatalf("insert user %s: %v", email, err)
		}
	}
	shopperID := userID(t, db, "shopper@example.com")
	adminID := userID(t, db, "other@example.com")

	return orders.NewService(orders.NewRepository(db), orders.Intake{Catalogue: testCatalogue()}, nil), db, shopperID, adminID
}

func storedStatus(t *testing.T, db database.DB, id int) string {
	t.Helper()
	rows, err := db.Query(context.Background(), `SELECT status FROM orders WHERE id = ?`, id)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("order %d is not in the table", id)
	}
	s, err := rows[0].String("status")
	if err != nil {
		t.Fatalf("scan status: %v", err)
	}
	return s
}

// A status transition must reach the row. The parameter order for a scoped
// write is easy to get wrong in a way that still reports success: the update
// matches no row, so the response is a re-read of the unchanged order.
func TestUpdateStatusReachesTheRow(t *testing.T) {
	ctx := context.Background()
	svc, db, shopperID, adminID := newWriteFixture(t)

	created, err := svc.Create(ctx, orderInput("shopper@example.com"), orders.Shopper(shopperID))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := svc.UpdateStatus(ctx, orders.Administrator(adminID), created.ID, orders.StatusConfirmed)
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if updated.Status != orders.StatusConfirmed {
		t.Errorf("returned status = %q, want confirmed", updated.Status)
	}
	if got := storedStatus(t, db, created.ID); got != string(orders.StatusConfirmed) {
		t.Errorf("stored status = %q, want confirmed", got)
	}

	// The lifecycle still guards, and the guard is now a named error.
	if _, err := svc.UpdateStatus(ctx, orders.Administrator(adminID), created.ID, orders.StatusConfirmed); !errors.Is(err, orders.ErrInvalidStatusTransition) {
		t.Errorf("re-confirming returned %v, want ErrInvalidStatusTransition", err)
	}
}

// Update must reach the row, replace the line items, and re-price them. The
// prices in the request are a request: the catalogue decides what the order
// costs, so a shopper editing their own order cannot reprice it.
func TestUpdateReachesTheRowAndRepricesIt(t *testing.T) {
	ctx := context.Background()
	svc, db, shopperID, _ := newWriteFixture(t)

	created, err := svc.Create(ctx, orderInput("shopper@example.com"), orders.Shopper(shopperID))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	changed := orderInput("shopper@example.com")
	changed.UserName = "Renamed"
	changed.Street = "9 New Street"
	changed.Total = 1
	changed.Items = []orders.OrderItemFields{
		{ProductID: 1, ProductName: "Cheap knock-off", Price: 1, Quantity: 3, Total: 3},
	}
	if _, err := svc.Update(ctx, orders.Shopper(shopperID), created.ID, changed); err != nil {
		t.Fatalf("Update: %v", err)
	}

	rows, err := db.Query(ctx, `SELECT user_name, street, total FROM orders WHERE id = ?`, created.ID)
	if err != nil {
		t.Fatalf("read order: %v", err)
	}
	name, _ := rows[0].String("user_name")
	street, _ := rows[0].String("street")
	total, _ := rows[0].Float("total")
	if name != "Renamed" || street != "9 New Street" {
		t.Errorf("stored order is (%q, %q), want (Renamed, 9 New Street)", name, street)
	}
	// The catalogue prices Sparklers at 2500, so three of them is 7500. The
	// request asked for 3 rupees.
	if total != 7500 {
		t.Errorf("stored total = %v, want 7500 (the client's price must be ignored)", total)
	}

	itemRows, err := db.Query(ctx, `SELECT product_name, price, quantity, total FROM order_items WHERE order_id = ?`, created.ID)
	if err != nil {
		t.Fatalf("read items: %v", err)
	}
	if len(itemRows) != 1 {
		t.Fatalf("order has %d line items, want 1 (the old set must be replaced)", len(itemRows))
	}
	itemName, _ := itemRows[0].String("product_name")
	itemPrice, _ := itemRows[0].Float("price")
	if itemName != "Sparklers" || itemPrice != 2500 {
		t.Errorf("line is (%q at %v), want (Sparklers at 2500)", itemName, itemPrice)
	}
}

// Delete must remove the order and its line items, and must not touch an
// order the caller does not own.
func TestDeleteRemovesTheOrderAndItsItems(t *testing.T) {
	ctx := context.Background()
	svc, db, shopperID, _ := newWriteFixture(t)
	shopper := orders.Shopper(shopperID)

	mine, err := svc.Create(ctx, orderInput("shopper@example.com"), shopper)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	other, err := svc.Create(ctx, orderInput("shopper@example.com"), shopper)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Delete(ctx, shopper, other.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	rows, err := db.Query(ctx, `SELECT COUNT(*) AS n FROM orders WHERE id = ?`, other.ID)
	if err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if n, _ := rows[0].Int("n"); n != 0 {
		t.Errorf("order %d survived the delete", other.ID)
	}
	items, err := db.Query(ctx, `SELECT COUNT(*) AS n FROM order_items WHERE order_id = ?`, other.ID)
	if err != nil {
		t.Fatalf("count items: %v", err)
	}
	if n, _ := items[0].Int("n"); n != 0 {
		t.Errorf("%d orphaned line items survived the delete", n)
	}

	// A delete aimed at somebody else's order must not damage it.
	if err := svc.Delete(ctx, orders.Shopper(9999), mine.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("cross-account Delete returned %v, want ErrNotFound", err)
	}
	survivor, err := svc.Get(ctx, shopper, mine.ID)
	if err != nil {
		t.Fatalf("the other order is gone: %v", err)
	}
	if len(survivor.Items) != 1 {
		t.Errorf("the surviving order lost its line items: %d", len(survivor.Items))
	}
}

// The invoice number is written through a scoped statement too.
// The response to a create is the row as stored, not the request that produced
// it. Echoing the input returned an order whose status was empty, because the
// default is applied by the database.
func TestCreateReturnsTheStoredRow(t *testing.T) {
	ctx := context.Background()
	svc, db, shopperID, _ := newWriteFixture(t)

	created, err := svc.Create(ctx, orderInput("shopper@example.com"), orders.Shopper(shopperID))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if created.Status != orders.StatusPending {
		t.Errorf("created status = %q, want the stored default %q", created.Status, orders.StatusPending)
	}
	if created.CreatedAt == "" {
		t.Error("created order has no createdAt; the response echoed the request")
	}
	if len(created.Items) != 1 || created.Items[0].ID == 0 {
		t.Errorf("created items = %+v, want one line with a stored id", created.Items)
	}
	if got := storedStatus(t, db, created.ID); got != string(orders.StatusPending) {
		t.Errorf("stored status = %q, want pending", got)
	}
}

// A payload may not set the fields the order module owns. `status` moves through
// UpdateStatus so the lifecycle and its event apply; `user_id` is the owner and
// comes from the session. Both used to be accepted in the body and then silently
// dropped by the write, so a caller who set them got a 200 and no change — which
// is worse than a refusal, because it looks like it worked.
func TestUpdateRefusesTheFieldsTheModuleOwns(t *testing.T) {
	ctx := context.Background()
	svc, db, shopperID, adminID := newWriteFixture(t)
	shopper := orders.Shopper(shopperID)

	created, err := svc.Create(ctx, orderInput("shopper@example.com"), shopper)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	spoofed := adminID
	cancelled := orders.StatusCancelled
	for _, c := range []struct {
		name  string
		apply func(in *orders.OrderInput)
	}{
		{"status", func(in *orders.OrderInput) { in.Status = cancelled }},
		{"userId", func(in *orders.OrderInput) { in.UserID = &spoofed }},
	} {
		input := orderInput("shopper@example.com")
		c.apply(&input)
		if _, err := svc.Update(ctx, shopper, created.ID, input); !errors.Is(err, orders.ErrImmutableFields) {
			t.Errorf("Update with %s set returned %v, want ErrImmutableFields", c.name, err)
		}
	}

	// Nothing moved.
	after, err := svc.Get(ctx, shopper, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if after.Status != orders.StatusPending {
		t.Errorf("status = %q, want pending", after.Status)
	}
	if after.UserID == nil || *after.UserID != shopperID {
		t.Errorf("owner = %v, want %d", after.UserID, shopperID)
	}
	if got := storedStatus(t, db, created.ID); got != string(orders.StatusPending) {
		t.Errorf("stored status = %q, want pending", got)
	}
}

// Each line item is one INSERT, and on D1 each of those is a separate HTTP round
// trip. An unbounded count on a route a guest may reach is a request to make the
// server do tens of thousands of calls.
func TestOversizedBasketIsRefused(t *testing.T) {
	ctx := context.Background()
	svc, db := newPricedFixture(t, testCatalogue())

	input := orderInput("shopper@example.com")
	input.Items = make([]orders.OrderItemFields, orders.MaxItemsPerOrder+1)
	for i := range input.Items {
		input.Items[i] = orders.OrderItemFields{ProductID: 1, ProductName: "Sparklers", Price: 2500, Quantity: 1, Total: 2500}
	}

	if _, err := svc.Create(ctx, input, orders.Guest()); !errors.Is(err, orders.ErrTooManyItems) {
		t.Fatalf("Create with %d items returned %v, want ErrTooManyItems", len(input.Items), err)
	}

	rows, err := db.Query(ctx, `SELECT COUNT(*) AS n FROM orders`)
	if err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if n, _ := rows[0].Int("n"); n != 0 {
		t.Errorf("%d orders were written for an oversized basket", n)
	}

	// Exactly at the limit is still accepted.
	atLimit := orderInput("shopper@example.com")
	atLimit.Items = make([]orders.OrderItemFields, orders.MaxItemsPerOrder)
	for i := range atLimit.Items {
		atLimit.Items[i] = orders.OrderItemFields{ProductID: 1, ProductName: "Sparklers", Price: 2500, Quantity: 1, Total: 2500}
	}
	if _, err := svc.Create(ctx, atLimit, orders.Guest()); err != nil {
		t.Errorf("Create with exactly %d items: %v", orders.MaxItemsPerOrder, err)
	}
}

func TestAssignInvoiceNumberReachesTheRow(t *testing.T) {
	ctx := context.Background()
	svc, _, shopperID, _ := newWriteFixture(t)
	shopper := orders.Shopper(shopperID)

	created, err := svc.Create(ctx, orderInput("shopper@example.com"), shopper)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.AssignInvoiceNumber(ctx, shopper, created.ID, "INV-0042"); err != nil {
		t.Fatalf("AssignInvoiceNumber: %v", err)
	}

	view, err := svc.InvoiceView(ctx, shopper, created.ID)
	if err != nil {
		t.Fatalf("InvoiceView: %v", err)
	}
	if view.Number != "INV-0042" {
		t.Errorf("invoice number = %q, want INV-0042", view.Number)
	}
}
