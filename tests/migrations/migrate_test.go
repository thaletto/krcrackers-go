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

	if err := migrations.Down(ctx, db); err != nil {
		t.Fatalf("Down: %v", err)
	}
	assertProductColumns(false, false)
}
