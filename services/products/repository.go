package products

import (
	"context"
	"fmt"

	"github.com/thaletto/krcrackers-go/database"
)

type Repository interface {
	Create(ctx context.Context, input ProductInput) (Product, error)
	List(ctx context.Context, limit, offset int) (ListProductsResponse, error)
	Get(ctx context.Context, id int) (Product, error)
	Update(ctx context.Context, id int, input ProductInput) (Product, error)
	Delete(ctx context.Context, id int) error
}

type repo struct {
	db database.DB
}

func NewRepository(db database.DB) Repository {
	return &repo{db: db}
}

func (r *repo) Create(ctx context.Context, input ProductInput) (Product, error) {
	res, err := r.db.Execute(ctx, `
		INSERT INTO products (name, price, brand, description, category, image, compare_price)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, input.Name, input.Price, input.Brand, input.Description, input.Category, input.Image, input.ComparePrice)
	if err != nil {
		return Product{}, fmt.Errorf("insert product: %w", err)
	}
	return productFromInput(int(res.LastInsertID), input), nil
}

func (r *repo) List(ctx context.Context, limit, offset int) (ListProductsResponse, error) {
	countRows, err := r.db.Query(ctx, `SELECT COUNT(*) AS total FROM products`)
	if err != nil {
		return ListProductsResponse{}, fmt.Errorf("count products: %w", err)
	}
	total := 0
	if len(countRows) > 0 {
		if v, err := countRows[0].Int("total"); err == nil {
			total = int(v)
		}
	}

	query := `
		SELECT id, name, price, brand, description, category, image, compare_price
		FROM products
		ORDER BY id
	`
	var queryArgs []any
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		queryArgs = append(queryArgs, limit, offset)
	}

	rows, err := r.db.Query(ctx, query, queryArgs...)
	if err != nil {
		return ListProductsResponse{}, fmt.Errorf("list products: %w", err)
	}

	var limitPtr, offsetPtr *int
	if limit > 0 {
		limitPtr = &limit
		offsetPtr = &offset
	}

	items := make([]Product, 0, len(rows))
	for _, row := range rows {
		p, err := rowToProduct(row)
		if err != nil {
			return ListProductsResponse{}, fmt.Errorf("scan product: %w", err)
		}
		items = append(items, p)
	}
	return ListProductsResponse{
		Items:  items,
		Total:  total,
		Limit:  limitPtr,
		Offset: offsetPtr,
	}, nil
}

func (r *repo) Get(ctx context.Context, id int) (Product, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, price, brand, description, category, image, compare_price
		FROM products WHERE id = ?
	`, id)
	if err != nil {
		return Product{}, fmt.Errorf("get product %d: %w", id, err)
	}
	if len(rows) == 0 {
		return Product{}, fmt.Errorf("product not found")
	}
	return rowToProduct(rows[0])
}

func (r *repo) Update(ctx context.Context, id int, input ProductInput) (Product, error) {
	res, err := r.db.Execute(ctx, `
		UPDATE products
		SET name = ?, price = ?, brand = ?, description = ?, category = ?, image = ?, compare_price = ?
		WHERE id = ?
	`, input.Name, input.Price, input.Brand, input.Description, input.Category, input.Image, input.ComparePrice, id)
	if err != nil {
		return Product{}, fmt.Errorf("update product %d: %w", id, err)
	}
	if res.RowsAffected == 0 {
		return Product{}, fmt.Errorf("product not found")
	}
	return productFromInput(id, input), nil
}

func (r *repo) Delete(ctx context.Context, id int) error {
	res, err := r.db.Execute(ctx, `DELETE FROM products WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete product %d: %w", id, err)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("product not found")
	}
	return nil
}

func productFromInput(id int, b ProductInput) Product {
	return Product{ID: id, ProductFields: b.ProductFields}
}

func rowToProduct(row database.Row) (Product, error) {
	id, err := row.Int("id")
	if err != nil {
		return Product{}, err
	}
	name, err := row.String("name")
	if err != nil {
		return Product{}, err
	}
	price, err := row.Float("price")
	if err != nil {
		return Product{}, err
	}
	brand, err := row.NullableString("brand")
	if err != nil {
		return Product{}, err
	}
	description, err := row.NullableString("description")
	if err != nil {
		return Product{}, err
	}
	category, err := row.String("category")
	if err != nil {
		return Product{}, err
	}
	image, err := row.NullableString("image")
	if err != nil {
		return Product{}, err
	}
	comparePrice, err := row.Float("compare_price")
	if err != nil {
		return Product{}, err
	}
	return Product{
		ID: int(id),
		ProductFields: ProductFields{
			Name:         name,
			Price:        price,
			Brand:        brand,
			Description:  description,
			Category:     category,
			Image:        image,
			ComparePrice: comparePrice,
		},
	}, nil
}
