package products

import (
	"context"
	"log"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/thaletto/krcrackers-go/database"
	"github.com/thaletto/krcrackers-go/dbconv"
)

type Service struct {
	db database.DB
}

// Product is the response shape (id populated by the server).
type Product struct {
	ID           int     `json:"id" example:"1"`
	Name         string  `json:"name" example:"Sneaker"`
	Price        float64 `json:"price" example:"99"`
	Brand        *string `json:"brand,omitempty" nullable:"true" example:"Acme"`
	Description  *string `json:"description,omitempty" nullable:"true" example:"Shoes"`
	Category     string  `json:"category" example:"footwear"`
	Image        *string `json:"image,omitempty" nullable:"true" example:"/s.png"`
	ComparePrice float64 `json:"comparePrice" example:"129"`
}

// ProductInput is the request body for create/update (no id).
type ProductInput struct {
	Name         string  `json:"name" required:"true" minLength:"1" example:"Sneaker"`
	Price        float64 `json:"price" required:"true" minimum:"0" example:"99"`
	Brand        *string `json:"brand,omitempty" nullable:"true" example:"Acme"`
	Description  *string `json:"description,omitempty" nullable:"true" example:"Shoes"`
	Category     string  `json:"category" required:"true" minLength:"1" example:"footwear"`
	Image        *string `json:"image,omitempty" nullable:"true" example:"/s.png"`
	ComparePrice float64 `json:"comparePrice" required:"true" minimum:"0" example:"129"`
}

type CreateProductInput struct {
	Body ProductInput
}
type CreateProductOutput struct {
	Body Product
}

type ListProductsOutput struct {
	Body []Product
}

type GetProductInput struct {
	ID int `path:"id" required:"true" minimum:"1" example:"1"`
}
type GetProductOutput struct {
	Body Product
}

type UpdateProductInput struct {
	ID   int `path:"id" required:"true" minimum:"1" example:"1"`
	Body ProductInput
}
type UpdateProductOutput struct {
	Body Product
}

type DeleteProductInput struct {
	ID int `path:"id" required:"true" minimum:"1" example:"1"`
}

func NewService(db database.DB) *Service {
	return &Service{db: db}
}

func (s *Service) RegisterRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-product",
		Method:      http.MethodPost,
		Path:        "/products",
		Summary:     "Create a product",
		Description: "Adds a new product to the catalog.",
		Tags:        []string{"products"},
	}, s.create)

	huma.Register(api, huma.Operation{
		OperationID: "list-products",
		Method:      http.MethodGet,
		Path:        "/products",
		Summary:     "List products",
		Description: "Returns all products, ordered by id.",
		Tags:        []string{"products"},
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID: "get-product",
		Method:      http.MethodGet,
		Path:        "/products/{id}",
		Summary:     "Get a product",
		Description: "Returns a single product by id.",
		Tags:        []string{"products"},
	}, s.get)

	huma.Register(api, huma.Operation{
		OperationID: "update-product",
		Method:      http.MethodPut,
		Path:        "/products/{id}",
		Summary:     "Update a product",
		Description: "Replaces an existing product. Returns 404 if the product does not exist.",
		Tags:        []string{"products"},
	}, s.update)

	huma.Register(api, huma.Operation{
		OperationID: "delete-product",
		Method:      http.MethodDelete,
		Path:        "/products/{id}",
		Summary:     "Delete a product",
		Description: "Removes a product from the catalog. Returns 404 if the product does not exist.",
		Tags:        []string{"products"},
	}, s.delete)
}

func (s *Service) create(ctx context.Context, in *CreateProductInput) (*CreateProductOutput, error) {
	b := in.Body
	res, err := s.db.Execute(ctx, `
		INSERT INTO products (name, price, brand, description, category, image, compare_price)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, b.Name, b.Price, b.Brand, b.Description, b.Category, b.Image, b.ComparePrice)
	if err != nil {
		log.Printf("insert product: %v", err)
		return nil, huma.Error500InternalServerError("failed to create product")
	}

	return &CreateProductOutput{Body: productFromInput(int(res.LastInsertID), b)}, nil
}

func (s *Service) list(ctx context.Context, _ *struct{}) (*ListProductsOutput, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, price, brand, description, category, image, compare_price
		FROM products
		ORDER BY id
	`)
	if err != nil {
		log.Printf("list products: %v", err)
		return nil, huma.Error500InternalServerError("failed to list products")
	}

	out := make([]Product, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToProduct(row))
	}
	return &ListProductsOutput{Body: out}, nil
}

func (s *Service) get(ctx context.Context, in *GetProductInput) (*GetProductOutput, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, price, brand, description, category, image, compare_price
		FROM products WHERE id = ?
	`, in.ID)
	if err != nil {
		log.Printf("get product %d: %v", in.ID, err)
		return nil, huma.Error500InternalServerError("failed to get product")
	}

	if len(rows) == 0 {
		return nil, huma.Error404NotFound("product not found")
	}

	return &GetProductOutput{Body: rowToProduct(rows[0])}, nil
}

func (s *Service) update(ctx context.Context, in *UpdateProductInput) (*UpdateProductOutput, error) {
	b := in.Body
	res, err := s.db.Execute(ctx, `
		UPDATE products
		SET name = ?, price = ?, brand = ?, description = ?, category = ?, image = ?, compare_price = ?
		WHERE id = ?
	`, b.Name, b.Price, b.Brand, b.Description, b.Category, b.Image, b.ComparePrice, in.ID)
	if err != nil {
		log.Printf("update product %d: %v", in.ID, err)
		return nil, huma.Error500InternalServerError("failed to update product")
	}

	if res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("product not found")
	}

	return &UpdateProductOutput{Body: productFromInput(in.ID, b)}, nil
}

func (s *Service) delete(ctx context.Context, in *DeleteProductInput) (*struct{}, error) {
	res, err := s.db.Execute(ctx, `DELETE FROM products WHERE id = ?`, in.ID)
	if err != nil {
		log.Printf("delete product %d: %v", in.ID, err)
		return nil, huma.Error500InternalServerError("failed to delete product")
	}
	if res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("product not found")
	}
	return nil, nil
}

func productFromInput(id int, b ProductInput) Product {
	return Product{
		ID:           id,
		Name:         b.Name,
		Price:        b.Price,
		Brand:        b.Brand,
		Description:  b.Description,
		Category:     b.Category,
		Image:        b.Image,
		ComparePrice: b.ComparePrice,
	}
}

func rowToProduct(row map[string]any) Product {
	return Product{
		ID:           dbconv.Int(row["id"]),
		Name:         dbconv.String(row["name"]),
		Price:        dbconv.Float(row["price"]),
		Brand:        dbconv.NullableString(row["brand"]),
		Description:  dbconv.NullableString(row["description"]),
		Category:     dbconv.String(row["category"]),
		Image:        dbconv.NullableString(row["image"]),
		ComparePrice: dbconv.Float(row["compare_price"]),
	}
}
