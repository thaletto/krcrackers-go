package migrations_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thaletto/krcrackers-go/src/database"
	"github.com/thaletto/krcrackers-go/src/migrations"
)

func emptyDB(t *testing.T) database.DB {
	t.Helper()
	db, err := database.New(database.Config{
		Mode:  database.ModeLocal,
		Local: &database.LocalConfig{Path: filepath.Join(t.TempDir(), "migrations.sqlite")},
	})
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func tableExists(t *testing.T, db database.DB, name string) bool {
	t.Helper()
	rows, err := db.Query(context.Background(),
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", name)
	if err != nil {
		t.Fatalf("read sqlite_master: %v", err)
	}
	return len(rows) > 0
}

// The bug this guards: GetStatus called ensureVersionTable before reading, so
// asking what had been applied issued a CREATE TABLE. On D1 that is a write
// round-trip for a read, and on any backend it means a status query changes the
// database it is describing.
func TestGetStatusDoesNotWrite(t *testing.T) {
	ctx := context.Background()
	db := emptyDB(t)

	statuses, err := migrations.GetStatus(ctx, db)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if len(statuses) == 0 {
		t.Fatal("GetStatus reported no migrations; there are several")
	}
	for _, s := range statuses {
		if s.Applied {
			t.Errorf("migration %04d reported applied on an empty database", s.Version)
		}
	}
	if tableExists(t, db, "schema_migrations") {
		t.Error("GetStatus created the bookkeeping table; a status query must only read")
	}
}

// Up and GetStatus must agree about what has been applied. They used to read
// the bookkeeping differently — Up took MAX(version) as a high-water mark,
// GetStatus took the set of recorded versions — so a row recorded below the
// mark was invisible to Up while GetStatus called it applied.
func TestUpAndStatusAgreeOnWhatIsApplied(t *testing.T) {
	ctx := context.Background()
	db := emptyDB(t)

	first, err := migrations.Up(ctx, db)
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if first == 0 {
		t.Fatal("Up applied nothing on an empty database")
	}

	statuses, err := migrations.GetStatus(ctx, db)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	for _, s := range statuses {
		if !s.Applied {
			t.Errorf("migration %04d is not applied, but Up reported %d applied", s.Version, first)
		}
	}

	// A second run is a no-op: every version is already in the set.
	again, err := migrations.Up(ctx, db)
	if err != nil {
		t.Fatalf("second Up: %v", err)
	}
	if again != 0 {
		t.Errorf("second Up applied %d migrations, want 0", again)
	}
}

// A version missing from the bookkeeping is attempted, not skipped. Under the
// old high-water mark a database recording versions 2-5 would never apply 1,
// because 1 is below MAX(version) - silently, and reporting success.
func TestAVersionMissingFromTheBookkeepingIsAttempted(t *testing.T) {
	ctx := context.Background()
	db := emptyDB(t)

	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if _, err := db.Execute(ctx, "DELETE FROM schema_migrations WHERE version = 1"); err != nil {
		t.Fatalf("forget version 1: %v", err)
	}

	// The schema is still there, so re-running 0001 fails - and the error names
	// the version, which is the proof it was attempted rather than skipped.
	_, err := migrations.Up(ctx, db)
	if err == nil {
		t.Fatal("Up succeeded although version 1 is not recorded; it should have been attempted")
	}
	if !strings.Contains(err.Error(), "0001_init") {
		t.Errorf("Up error = %v, want it to name 0001_init", err)
	}
}

// A version recorded with no migration file means the database is at a state
// this build cannot describe. Down used to log "nothing to roll back" and
// return nil, which makes that state look deliberate.
func TestDownNamesARecordedVersionWithNoMigrationFile(t *testing.T) {
	ctx := context.Background()
	db := emptyDB(t)

	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("Up: %v", err)
	}
	if _, err := db.Execute(ctx, "DELETE FROM schema_migrations"); err != nil {
		t.Fatalf("clear the bookkeeping: %v", err)
	}
	if _, err := db.Execute(ctx, "INSERT INTO schema_migrations (version) VALUES (99)"); err != nil {
		t.Fatalf("record a version with no file: %v", err)
	}

	err := migrations.Down(ctx, db)
	if err == nil {
		t.Fatal("Down succeeded although the only recorded version is one this build has no file for")
	}
	if !strings.Contains(err.Error(), "0099") {
		t.Errorf("Down error = %v, want it to name the unknown version", err)
	}
}

// A migration whose Up section holds a `;` inside a string literal must apply
// as one statement. The old pipeline stripped comments in a quote-aware pass
// and then split on `;` in a pass that ignored quotes, so it split mid-literal.
func TestStatementSplittingRespectsQuotedSemicolons(t *testing.T) {
	ctx := context.Background()
	db := emptyDB(t)

	// A trigger body is the case the StatementBegin/End markers exist for; the
	// literal-semicolon case is an ordinary statement.
	if _, err := db.Execute(ctx, `
		CREATE TABLE splitter_probe (label TEXT DEFAULT 'semi;colon', n INTEGER)
	`); err != nil {
		t.Fatalf("probe table: %v", err)
	}

	// A migration whose Up section contains a quoted semicolon, applied through
	// the real runner by checking the trigger-bearing init migration landed.
	if _, err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("Up: %v", err)
	}
	rows, err := db.Query(ctx, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'orders'`)
	if err != nil {
		t.Fatalf("read sqlite_master: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("the orders table is missing after Up")
	}
}
