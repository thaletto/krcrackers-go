package database_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/thaletto/krcrackers-go/src/database"
)

func newBenchDB(b *testing.B) database.DB {
	b.Helper()
	db, err := database.New(database.Config{
		Mode:  database.ModeLocal,
		Local: &database.LocalConfig{Path: filepath.Join(b.TempDir(), "bench.sqlite")},
	})
	if err != nil {
		b.Fatalf("database.New: %v", err)
	}
	b.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if _, err := db.Execute(ctx, `CREATE TABLE bench (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, price REAL NOT NULL)`); err != nil {
		b.Fatalf("create: %v", err)
	}
	for i := 0; i < 100; i++ {
		if _, err := db.Execute(ctx, `INSERT INTO bench (name, price) VALUES (?, ?)`, "item", 9.99); err != nil {
			b.Fatalf("seed: %v", err)
		}
	}
	return db
}

func BenchmarkSQLiteQuery100Rows(b *testing.B) {
	db := newBenchDB(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := db.Query(ctx, `SELECT id, name, price FROM bench ORDER BY id`)
		if err != nil {
			b.Fatalf("query: %v", err)
		}
		for _, r := range rows {
			if _, err := r.Int("id"); err != nil {
				b.Fatalf("Int: %v", err)
			}
			if _, err := r.String("name"); err != nil {
				b.Fatalf("String: %v", err)
			}
			if _, err := r.Float("price"); err != nil {
				b.Fatalf("Float: %v", err)
			}
		}
	}
}
