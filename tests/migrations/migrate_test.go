package migrations_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thaletto/krcrackers-go/src/database"
	"github.com/thaletto/krcrackers-go/src/migrations"
)

func newTestDB(t *testing.T) database.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := database.New(database.Config{
		Mode:  database.ModeLocal,
		Local: &database.LocalConfig{Path: filepath.Join(dir, "migrate.sqlite")},
	})
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestStatusStringFormatsAppliedAndPending(t *testing.T) {
	pending := migrations.Status{Version: 7, Name: "add_widgets", Applied: false}
	if got, want := pending.String(), "0007_add_widgets\tpending"; got != want {
		t.Errorf("pending: got %q, want %q", got, want)
	}

	applied := migrations.Status{Version: 7, Name: "add_widgets", Applied: true}
	if got, want := applied.String(), "0007_add_widgets\tapplied"; got != want {
		t.Errorf("applied: got %q, want %q", got, want)
	}
}

func TestGetStatusOnFreshDatabase(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	statuses, err := migrations.GetStatus(ctx, db)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if len(statuses) == 0 {
		t.Fatal("expected at least one migration to be discovered")
	}
	for _, s := range statuses {
		if s.Applied {
			t.Errorf("fresh DB: migration %d should be pending, got applied", s.Version)
		}
	}
}

func TestUpAppliesAllMigrations(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	applied, err := migrations.Up(ctx, db)
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if applied < 1 {
		t.Fatalf("expected at least one migration applied, got %d", applied)
	}

	statuses, err := migrations.GetStatus(ctx, db)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	for _, s := range statuses {
		if !s.Applied {
			t.Errorf("migration %d should be applied after Up", s.Version)
		}
	}
}

func TestUpIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("first Up: %v", err)
	}

	applied, err := migrations.Up(ctx, db)
	if err != nil {
		t.Fatalf("second Up: %v", err)
	}
	if applied != 0 {
		t.Errorf("second Up should apply 0, applied %d", applied)
	}
}

func TestDownRollsBackMostRecent(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("Up: %v", err)
	}

	before, err := migrations.GetStatus(ctx, db)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	allAppliedBefore := true
	for _, s := range before {
		if !s.Applied {
			allAppliedBefore = false
			break
		}
	}
	if !allAppliedBefore {
		t.Fatal("precondition: all migrations should be applied")
	}

	if err := migrations.Down(ctx, db); err != nil {
		t.Fatalf("Down: %v", err)
	}

	after, err := migrations.GetStatus(ctx, db)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if after[len(after)-1].Applied {
		t.Errorf("latest migration should no longer be applied")
	}
}

func TestDownOnFreshDatabaseIsNoop(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	if err := migrations.Down(ctx, db); err != nil {
		t.Fatalf("Down on fresh db: %v", err)
	}
}

func hasColumn(t *testing.T, db database.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(context.Background(), `PRAGMA table_info(`+table+`)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
	}
	for _, r := range rows {
		if name, _ := r.String("name"); name == column {
			return true
		}
	}
	return false
}

func hasIndex(t *testing.T, db database.DB, index string) bool {
	t.Helper()
	rows, err := db.Query(context.Background(), `SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, index)
	if err != nil {
		t.Fatalf("query sqlite_master for index: %v", err)
	}
	return len(rows) > 0
}

// 0004 carries explanatory comments inside its Up and Down sections, which is
// the natural way to write a migration and exactly the case that used to be
// mishandled: a chunk opening with a comment also contains the statement after
// it, so skipping the chunk dropped that statement while the migration still
// reported as applied. This checks each of its three statements landed.
func TestGoogleLinkingMigrationAppliesEveryStatement(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if !hasColumn(t, db, "users", "google_id") {
		t.Error("users.google_id missing: the ALTER statement was dropped")
	}
	if !hasIndex(t, db, "users_google_id_unique") {
		t.Error("users_google_id_unique missing: the CREATE INDEX statement was dropped")
	}
}

// The backfill is a third statement that can be dropped independently of the
// other two, and its absence silently breaks sign-in for existing Google
// accounts. Rolling back, planting a legacy row, and re-applying exercises it.
func TestGoogleLinkingMigrationBackfillsLegacyGoogleAccounts(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("Up: %v", err)
	}
	// Roll back one step at a time: Down only reverts the newest migration, so
	// a single step would land on whatever was added after 0004 and leave
	// google_id in place.
	for hasColumn(t, db, "users", "google_id") {
		if err := migrations.Down(ctx, db); err != nil {
			t.Fatalf("migrations.Down: %v", err)
		}
	}
	if hasColumn(t, db, "users", "google_id") {
		t.Fatal("users.google_id should be gone after the rollback")
	}

	// A row in the shape migration 0004 predates: Google provider, subject in
	// the legacy auth_provider_id column.
	if _, err := db.Execute(ctx, `
		INSERT INTO users (email, name, auth_provider, auth_provider_id, password_hash, role)
		VALUES (?, ?, 'google', ?, '', 'customer')
	`, "legacy@example.com", "Legacy", "legacy-subject-1"); err != nil {
		t.Fatalf("insert legacy user: %v", err)
	}

	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("re-apply Up: %v", err)
	}

	rows, err := db.Query(ctx, `SELECT google_id FROM users WHERE email = ?`, "legacy@example.com")
	if err != nil {
		t.Fatalf("query backfilled user: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 legacy user, got %d", len(rows))
	}
	got, err := rows[0].String("google_id")
	if err != nil {
		t.Fatalf("read google_id: %v", err)
	}
	if got != "legacy-subject-1" {
		t.Errorf("backfilled google_id: got %q, want %q (the UPDATE statement was dropped?)", got, "legacy-subject-1")
	}
}

func TestUpCreatesExpectedTables(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("Up: %v", err)
	}

	rows, err := db.Query(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}

	tables := make(map[string]bool)
	for _, r := range rows {
		name, _ := r.String("name")
		tables[name] = true
	}

	for _, want := range []string{"users", "orders", "order_items", "products", "customer_addresses", "refresh_tokens"} {
		if !tables[want] {
			tablesList := make([]string, 0, len(tables))
			for k := range tables {
				tablesList = append(tablesList, k)
			}
			t.Errorf("expected table %q to exist; tables present: %s", want, strings.Join(tablesList, ", "))
		}
	}
}

func TestProductMetadataMigrationUpAndDown(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("Up: %v", err)
	}

	assertProductColumns := func(wantRating, wantDelivery bool) {
		t.Helper()
		rows, err := db.Query(ctx, `PRAGMA table_info(products)`)
		if err != nil {
			t.Fatalf("PRAGMA table_info: %v", err)
		}
		columns := make(map[string]bool)
		for _, row := range rows {
			name, err := row.String("name")
			if err != nil {
				t.Fatalf("column name: %v", err)
			}
			columns[name] = true
		}
		if columns["rating"] != wantRating {
			t.Errorf("rating column present=%v, want %v", columns["rating"], wantRating)
		}
		if columns["delivery"] != wantDelivery {
			t.Errorf(
				"delivery column present=%v, want %v",
				columns["delivery"],
				wantDelivery,
			)
		}
	}

	assertProductColumns(true, true)

	// Roll back one step at a time until the product metadata columns are gone.
	// The step count is not hardcoded: migrations.Down only rolls back the
	// latest version, so pinning a number would silently change what this test
	// covers every time a migration is added above 0002.
	for range 10 {
		rows, err := db.Query(ctx, `PRAGMA table_info(products)`)
		if err != nil {
			t.Fatalf("PRAGMA table_info: %v", err)
		}
		stillThere := false
		for _, row := range rows {
			if name, err := row.String("name"); err == nil && name == "rating" {
				stillThere = true
			}
		}
		if !stillThere {
			break
		}
		if err := migrations.Down(ctx, db); err != nil {
			t.Fatalf("migrations.Down: %v", err)
		}
	}

	assertProductColumns(false, false)
}

// Orders written before the ownership fix carry a NULL user_id, which is why
// they showed up in the admin dashboard but not in the shopper's own history.
// Rolling back, planting rows in the pre-fix shape, and re-applying exercises
// the backfill: an order whose email matches an account gets linked, one whose
// email matches nobody stays a guest order.
func TestGuestOrdersMigrationBackfillsOwnershipByEmail(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if err := migrations.Down(ctx, db); err != nil {
		t.Fatalf("Down: %v", err)
	}

	if _, err := db.Execute(ctx, `
		INSERT INTO users (email, name, auth_provider, password_hash, role)
		VALUES ('Shopper@Example.com', 'Shopper', 'email', '', 'customer')
	`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	// Account emails are not normalised on the way in, so the match has to be
	// case-insensitive: this order is stored with different casing.
	plantGuestOrder(t, db, "shopper@example.com")
	plantGuestOrder(t, db, "walkup@example.com")

	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("re-apply Up: %v", err)
	}

	owner := orderOwner(t, db, "shopper@example.com")
	if owner == nil {
		t.Error("order matching an account has user_id = NULL, want the account id (backfill skipped?)")
	} else if *owner != 1 {
		t.Errorf("order matching an account has user_id = %d, want 1", *owner)
	}
	if got := orderOwner(t, db, "walkup@example.com"); got != nil {
		t.Errorf("order matching no account has user_id = %v, want NULL", *got)
	}
}

func plantGuestOrder(t *testing.T, db database.DB, email string) {
	t.Helper()
	_, err := db.Execute(context.Background(), `
		INSERT INTO orders (user_name, email, phone, street, town_or_city, state, pincode, delivery_region, delivery_location, total)
		VALUES ('Shopper', ?, '9999999999', '1 Test Street', 'Mumbai', 'Maharashtra', '400001', 'West', 'Mumbai', 2500)
	`, email)
	if err != nil {
		t.Fatalf("plant guest order for %s: %v", email, err)
	}
}

func orderOwner(t *testing.T, db database.DB, email string) *int {
	t.Helper()
	// user_id IS NULL is asked of SQL rather than inferred from Int: a NULL
	// column is not reliably reported as an error by the typed accessors.
	rows, err := db.Query(context.Background(),
		`SELECT user_id, user_id IS NULL AS unlinked FROM orders WHERE email = ?`, email)
	if err != nil {
		t.Fatalf("query order for %s: %v", email, err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 order for %s, got %d", email, len(rows))
	}
	if unlinked, err := rows[0].Int("unlinked"); err != nil || unlinked != 0 {
		return nil
	}
	id, err := rows[0].Int("user_id")
	if err != nil {
		t.Fatalf("read user_id: %v", err)
	}
	owner := int(id)
	return &owner
}
