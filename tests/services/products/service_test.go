package products_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/thaletto/krcrackers-go/src/database"
	"github.com/thaletto/krcrackers-go/src/migrations"
	"github.com/thaletto/krcrackers-go/src/services/products"
)

func productService(t *testing.T) *products.Service {
	t.Helper()

	db, err := database.New(database.Config{
		Mode: database.ModeLocal,
		Local: &database.LocalConfig{
			Path: filepath.Join(t.TempDir(), "products.sqlite"),
		},
	})
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := migrations.Up(context.Background(), db); err != nil {
		t.Fatalf("migrations.Up: %v", err)
	}

	return products.NewService(products.NewRepository(db), nil)
}

func TestProductMetadataRoundTrip(t *testing.T) {
	ctx := context.Background()
	svc := productService(t)
	rating := 4.8
	delivery := "  Delivery in 2 days  "

	created, err := svc.Create(ctx, products.ProductInput{
		ProductFields: products.ProductFields{
			Name:         "Celebration Gift Box",
			Price:        1499,
			ComparePrice: 1999,
			Category:     "Gift Boxes",
			Rating:       &rating,
			Delivery:     &delivery,
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Rating == nil || *created.Rating != rating {
		t.Fatalf("created rating: got %v, want %v", created.Rating, rating)
	}
	if created.Delivery == nil || *created.Delivery != "Delivery in 2 days" {
		t.Fatalf("created delivery: got %v, want trimmed value", created.Delivery)
	}

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Rating == nil || *got.Rating != rating {
		t.Errorf("get rating: got %v, want %v", got.Rating, rating)
	}
	if got.Delivery == nil || *got.Delivery != "Delivery in 2 days" {
		t.Errorf("get delivery: got %v, want Delivery in 2 days", got.Delivery)
	}

	listed, err := svc.List(ctx, 20, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].Rating == nil {
		t.Fatalf("list metadata: got %+v", listed.Items)
	}

	searched, err := svc.Search(ctx, products.Filter{Query: "Celebration"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(searched.Items) != 1 || searched.Items[0].Delivery == nil {
		t.Fatalf("search metadata: got %+v", searched.Items)
	}

	byID, err := svc.GetByIDs(ctx, []int{created.ID})
	if err != nil {
		t.Fatalf("GetByIDs: %v", err)
	}
	if len(byID) != 1 || byID[0].Rating == nil || byID[0].Delivery == nil {
		t.Fatalf("get by IDs metadata: got %+v", byID)
	}
}

func TestProductMetadataMayBeOmittedAndCleared(t *testing.T) {
	ctx := context.Background()
	svc := productService(t)
	rating := 4.2
	delivery := "Free delivery"

	created, err := svc.Create(ctx, products.ProductInput{
		ProductFields: products.ProductFields{
			Name:         "Flower Pots",
			Price:        499,
			ComparePrice: 499,
			Category:     "Flower Pots",
			Rating:       &rating,
			Delivery:     &delivery,
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	emptyDelivery := "  "
	updated, err := svc.Update(ctx, created.ID, products.ProductInput{
		ProductFields: products.ProductFields{
			Name:         created.Name,
			Price:        created.Price,
			ComparePrice: created.ComparePrice,
			Category:     created.Category,
			Delivery:     &emptyDelivery,
		},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Rating != nil || updated.Delivery != nil {
		t.Fatalf(
			"updated metadata: got rating=%v delivery=%v, want nil",
			updated.Rating,
			updated.Delivery,
		)
	}

	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Rating != nil || got.Delivery != nil {
		t.Fatalf(
			"stored metadata: got rating=%v delivery=%v, want nil",
			got.Rating,
			got.Delivery,
		)
	}
}

func TestProductRatingValidation(t *testing.T) {
	svc := productService(t)

	for _, rating := range []float64{-0.1, 5.1} {
		_, err := svc.Create(context.Background(), products.ProductInput{
			ProductFields: products.ProductFields{
				Name:     "Invalid rating",
				Price:    100,
				Category: "Test",
				Rating:   &rating,
			},
		})
		if err == nil {
			t.Errorf("rating %v: expected validation error", rating)
		}
	}
}

// The bug this guards: an unrecognised sort fell through a switch to the
// default `id DESC` ordering, so a caller asking for `price_ascd` silently got
// a different order instead of being told the value is wrong.
//
// It fails on the pre-fix code: the search returned products rather than
// ErrInvalidSort.
func TestUnsupportedSortIsRejected(t *testing.T) {
	ctx := context.Background()
	svc := productService(t)

	if _, err := svc.Create(ctx, products.ProductInput{
		ProductFields: products.ProductFields{Name: "Sparklers", Price: 100, Category: "Fireworks"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err := svc.Search(ctx, products.Filter{Sort: "price_ascd"})
	if !errors.Is(err, products.ErrInvalidSort) {
		t.Fatalf("Search with a typo'd sort returned %v, want ErrInvalidSort", err)
	}
}

// The supported orderings are the whole contract, so they are pinned rather
// than discovered: each must actually order the results it claims to.
func TestSupportedSortsOrderTheResults(t *testing.T) {
	ctx := context.Background()
	svc := productService(t)

	for _, p := range []struct {
		name  string
		price float64
	}{{"Cheap", 100}, {"Mid", 200}, {"Dear", 300}} {
		if _, err := svc.Create(ctx, products.ProductInput{
			ProductFields: products.ProductFields{Name: p.name, Price: p.price, Category: "Fireworks"},
		}); err != nil {
			t.Fatalf("Create %s: %v", p.name, err)
		}
	}

	for _, c := range []struct {
		sort string
		want []string
	}{
		{"price_asc", []string{"Cheap", "Mid", "Dear"}},
		{"price_desc", []string{"Dear", "Mid", "Cheap"}},
		// The default is newest first, which for ascending ids is Dear, Mid, Cheap.
		{"", []string{"Dear", "Mid", "Cheap"}},
	} {
		got, err := svc.Search(ctx, products.Filter{Category: "Fireworks", Sort: c.sort})
		if err != nil {
			t.Fatalf("Search(sort=%q): %v", c.sort, err)
		}
		names := make([]string, 0, len(got.Items))
		for _, p := range got.Items {
			names = append(names, p.Name)
		}
		if len(names) != len(c.want) {
			t.Fatalf("Search(sort=%q) returned %v, want %v", c.sort, names, c.want)
		}
		for i := range names {
			if names[i] != c.want[i] {
				t.Errorf("Search(sort=%q) = %v, want %v", c.sort, names, c.want)
				break
			}
		}
	}
}
