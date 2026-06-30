package database_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/thaletto/krcrackers-go/src/database"
)

func newTestDB(t *testing.T) database.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := database.New(database.Config{
		Mode:  database.ModeLocal,
		Local: &database.LocalConfig{Path: filepath.Join(dir, "test.sqlite")},
	})
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestLocalModeOpensAndCloses(t *testing.T) {
	db := newTestDB(t)
	if err := db.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestExecuteAndQueryRoundtrip(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	if _, err := db.Execute(ctx, `CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	res, err := db.Execute(ctx, `INSERT INTO t (name) VALUES (?)`, "alice")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if res.LastInsertID != 1 {
		t.Errorf("LastInsertID: got %d, want 1", res.LastInsertID)
	}

	rows, err := db.Query(ctx, `SELECT id, name FROM t WHERE name = ?`, "alice")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: got %d, want 1", len(rows))
	}

	id, err := rows[0].Int("id")
	if err != nil {
		t.Fatalf("Int: %v", err)
	}
	if id != 1 {
		t.Errorf("id: got %d, want 1", id)
	}
	name, err := rows[0].String("name")
	if err != nil {
		t.Fatalf("String: %v", err)
	}
	if name != "alice" {
		t.Errorf("name: got %q, want alice", name)
	}
}

func TestNullableStringHandlesNullAndValue(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	if _, err := db.Execute(ctx, `CREATE TABLE t (note TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.Execute(ctx, `INSERT INTO t (note) VALUES (NULL)`); err != nil {
		t.Fatalf("insert null: %v", err)
	}
	if _, err := db.Execute(ctx, `INSERT INTO t (note) VALUES (?)`, "hello"); err != nil {
		t.Fatalf("insert value: %v", err)
	}

	rows, err := db.Query(ctx, `SELECT note FROM t ORDER BY rowid`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows: got %d, want 2", len(rows))
	}

	gotNull, err := rows[0].NullableString("note")
	if err != nil {
		t.Fatalf("NullableString on null: %v", err)
	}
	if gotNull != nil {
		t.Errorf("null row: got %v, want nil", *gotNull)
	}

	gotVal, err := rows[1].NullableString("note")
	if err != nil {
		t.Fatalf("NullableString on value: %v", err)
	}
	if gotVal == nil || *gotVal != "hello" {
		t.Errorf("value row: got %v, want hello", gotVal)
	}
}

func TestTypeErrorOnWrongAccessor(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	if _, err := db.Execute(ctx, `CREATE TABLE t (n INTEGER)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.Execute(ctx, `INSERT INTO t (n) VALUES (42)`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	rows, err := db.Query(ctx, `SELECT n FROM t`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	_, err = rows[0].String("n")
	var typeErr *database.TypeError
	if !errors.As(err, &typeErr) {
		t.Fatalf("expected *TypeError, got %T (%v)", err, err)
	}
	if typeErr.Column != "n" {
		t.Errorf("column: got %q, want n", typeErr.Column)
	}
}

func TestFloatAccessorReadsFloatColumn(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	if _, err := db.Execute(ctx, `CREATE TABLE t (price REAL)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.Execute(ctx, `INSERT INTO t (price) VALUES (?)`, 1.25); err != nil {
		t.Fatalf("insert: %v", err)
	}

	rows, err := db.Query(ctx, `SELECT price FROM t`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	got, err := rows[0].Float("price")
	if err != nil {
		t.Fatalf("Float: %v", err)
	}
	if got != 1.25 {
		t.Errorf("price: got %v, want 1.25", got)
	}
}

func TestBeginCommitPersists(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	if _, err := db.Execute(ctx, `CREATE TABLE t (v TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Execute(ctx, `INSERT INTO t (v) VALUES (?)`, "committed"); err != nil {
		t.Fatalf("tx insert: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	rows, err := db.Query(ctx, `SELECT v FROM t`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: got %d, want 1", len(rows))
	}
	v, _ := rows[0].String("v")
	if v != "committed" {
		t.Errorf("v: got %q, want committed", v)
	}
}

func TestBeginRollbackDiscards(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)

	if _, err := db.Execute(ctx, `CREATE TABLE t (v TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Execute(ctx, `INSERT INTO t (v) VALUES (?)`, "doomed"); err != nil {
		t.Fatalf("tx insert: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	rows, err := db.Query(ctx, `SELECT v FROM t`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows after rollback: got %d, want 0", len(rows))
	}
}

func TestInvalidModeReturnsError(t *testing.T) {
	_, err := database.New(database.Config{Mode: database.Mode("nope")})
	if err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

func TestLocalModeMissingPathReturnsError(t *testing.T) {
	_, err := database.New(database.Config{Mode: database.ModeLocal})
	if err == nil {
		t.Fatal("expected error when LocalConfig is missing")
	}
}
