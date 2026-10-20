package products

import (
	"context"
	"fmt"
	"strings"

	"github.com/thaletto/krcrackers-go/src/database"
	apperrors "github.com/thaletto/krcrackers-go/src/errors"
)

// productColumns is the single projection of the products table. Every read
// uses it, so the column list and rowToProduct cannot drift apart.
const productColumns = `id, name, price, brand, description, category, image, compare_price, rating, delivery`

// Repository defines the data access interface for products. It owns the SQL,
// the row mapping and the pagination envelope; it holds no product rules and
// publishes no events.
type Repository interface {
	Create(ctx context.Context, input ProductInput) (Product, error)
	List(ctx context.Context, limit, offset int) (ListProductsResponse, error)
	Search(ctx context.Context, filter Filter) (ListProductsResponse, error)
	Get(ctx context.Context, id int) (Product, error)
	GetByIDs(ctx context.Context, ids []int) ([]Product, error)
	Update(ctx context.Context, id int, input ProductInput) (Product, error)
	Delete(ctx context.Context, id int) error
}

type repo struct {
	db database.DB
}

// NewRepository returns a new products repository backed by the given database.
func NewRepository(db database.DB) Repository {
	return &repo{db: db}
}

func (r *repo) Create(ctx context.Context, input ProductInput) (Product, error) {
	res, err := r.db.Execute(ctx, `
		INSERT INTO products (name, price, brand, description, category, image, compare_price, rating, delivery)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, input.Name, input.Price, input.Brand, input.Description, input.Category, input.Image, input.ComparePrice, input.Rating, input.Delivery)
	if err != nil {
		return Product{}, fmt.Errorf("insert product: %w", err)
	}
	// Re-read rather than echoing the input: the response should be the row as
	// stored, and the caller cannot know its id.
	return r.Get(ctx, int(res.LastInsertID))
}

func (r *repo) List(ctx context.Context, limit, offset int) (ListProductsResponse, error) {
	query := `SELECT ` + productColumns + `, COUNT(*) OVER() AS total FROM products ORDER BY id`
	var args []any
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}
	return r.page(ctx, query, args, limit, offset)
}

func (r *repo) Search(ctx context.Context, filter Filter) (ListProductsResponse, error) {
	// A non-empty query goes through FTS5; everything else is a plain filter.
	// Both paths return the same page shape.
	from := "products p"
	var where []string
	var args []any

	if filter.Query != "" {
		from += " JOIN products_fts ON p.id = products_fts.rowid"
		where = append(where, "products_fts MATCH ?")
		args = append(args, filter.Query)
	}
	if filter.Category != "" {
		where = append(where, "p.category = ?")
		args = append(args, filter.Category)
	}
	if filter.Brand != "" {
		where = append(where, "p.brand = ?")
		args = append(args, filter.Brand)
	}
	if filter.MinPrice > 0 {
		where = append(where, "p.price >= ?")
		args = append(args, filter.MinPrice)
	}
	if filter.MaxPrice > 0 {
		where = append(where, "p.price <= ?")
		args = append(args, filter.MaxPrice)
	}

	orderBy, err := orderByClause(filter.Sort)
	if err != nil {
		return ListProductsResponse{}, err
	}

	query := `SELECT p.id, p.name, p.price, p.brand, p.description, p.category, p.image, p.compare_price, p.rating, p.delivery, COUNT(*) OVER() AS total FROM ` + from
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY " + orderBy

	if filter.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, filter.Limit, filter.Offset)
	}
	return r.page(ctx, query, args, filter.Limit, filter.Offset)
}

// page runs a listing query and shapes it into the paginated response. The
// pagination echo is built here so every list endpoint reports it the same way.
func (r *repo) page(ctx context.Context, query string, args []any, limit, offset int) (ListProductsResponse, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return ListProductsResponse{}, fmt.Errorf("list products: %w", err)
	}

	total := 0
	if len(rows) > 0 {
		if v, err := rows[0].Int("total"); err == nil {
			total = int(v)
		}
	}

	items := make([]Product, 0, len(rows))
	for _, row := range rows {
		p, err := rowToProduct(row)
		if err != nil {
			return ListProductsResponse{}, fmt.Errorf("scan product: %w", err)
		}
		items = append(items, p)
	}

	var limitPtr, offsetPtr *int
	if limit > 0 {
		l, o := limit, offset
		limitPtr, offsetPtr = &l, &o
	}
	return ListProductsResponse{Items: items, Total: total, Limit: limitPtr, Offset: offsetPtr}, nil
}

func (r *repo) Get(ctx context.Context, id int) (Product, error) {
	rows, err := r.db.Query(ctx, `SELECT `+productColumns+` FROM products WHERE id = ?`, id)
	if err != nil {
		return Product{}, fmt.Errorf("get product %d: %w", id, err)
	}
	if len(rows) == 0 {
		return Product{}, fmt.Errorf("product %d: %w", id, apperrors.ErrNotFound)
	}
	return rowToProduct(rows[0])
}

func (r *repo) GetByIDs(ctx context.Context, ids []int) ([]Product, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := r.db.Query(ctx, `
		SELECT `+productColumns+`
		FROM products WHERE id IN (`+strings.Join(placeholders, ",")+`)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("get products by ids: %w", err)
	}
	products := make([]Product, 0, len(rows))
	for _, row := range rows {
		p, err := rowToProduct(row)
		if err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, nil
}

func (r *repo) Update(ctx context.Context, id int, input ProductInput) (Product, error) {
	res, err := r.db.Execute(ctx, `
		UPDATE products
		SET name = ?, price = ?, brand = ?, description = ?, category = ?, image = ?, compare_price = ?, rating = ?, delivery = ?
		WHERE id = ?
	`, input.Name, input.Price, input.Brand, input.Description, input.Category, input.Image, input.ComparePrice, input.Rating, input.Delivery, id)
	if err != nil {
		return Product{}, fmt.Errorf("update product %d: %w", id, err)
	}
	if res.RowsAffected == 0 {
		return Product{}, fmt.Errorf("product %d: %w", id, apperrors.ErrNotFound)
	}
	return r.Get(ctx, id)
}

func (r *repo) Delete(ctx context.Context, id int) error {
	res, err := r.db.Execute(ctx, `DELETE FROM products WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete product %d: %w", id, err)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("product %d: %w", id, apperrors.ErrNotFound)
	}
	return nil
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
	rating, err := row.NullableFloat("rating")
	if err != nil {
		return Product{}, err
	}
	delivery, err := row.NullableString("delivery")
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
			Rating:       rating,
			Delivery:     delivery,
		},
	}, nil
}
