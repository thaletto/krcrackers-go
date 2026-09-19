package products_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/thaletto/krcrackers-go/src/database"
	"github.com/thaletto/krcrackers-go/src/migrations"
	"github.com/thaletto/krcrackers-go/src/services/products"
)

func productServiceBench(b *testing.B) *products.Service {
	b.Helper()
	db, err := database.New(database.Config{
		Mode:  database.ModeLocal,
		Local: &database.LocalConfig{Path: filepath.Join(b.TempDir(), "products_bench.sqlite")},
	})
	if err != nil {
		b.Fatalf("database.New: %v", err)
	}
	b.Cleanup(func() { _ = db.Close() })
	if _, err := migrations.Up(context.Background(), db); err != nil {
		b.Fatalf("migrations.Up: %v", err)
	}
	svc := products.NewService(db, nil)
	seedProducts(b, svc, 100)
	return svc
}

func seedProducts(b *testing.B, svc *products.Service, n int) {
	b.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		_, err := svc.Create(ctx, products.ProductInput{
			ProductFields: products.ProductFields{
				Name:         fmt.Sprintf("Benchmark Cracker %d", i),
				Price:        99.99,
				ComparePrice: 129.99,
				Category:     "Benchmark",
			},
		})
		if err != nil {
			b.Fatalf("seed: %v", err)
		}
	}
}

func BenchmarkProductsList50(b *testing.B) {
	svc := productServiceBench(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.List(ctx, 50, 0); err != nil {
			b.Fatalf("List: %v", err)
		}
	}
}

func BenchmarkProductsSearch(b *testing.B) {
	svc := productServiceBench(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.Search(ctx, products.Filter{Query: "Benchmark", Limit: 20}); err != nil {
			b.Fatalf("Search: %v", err)
		}
	}
}
