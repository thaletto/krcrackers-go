package products

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/thaletto/krcrackers-go/database"
)

type Service struct {
	db database.DB
}

type ProductFields struct {
	Name         string   `json:"name"`
	Price        float64  `json:"price"`
	Brand        *string  `json:"brand,omitempty"`
	Description  *string  `json:"description,omitempty"`
	Category     string   `json:"category"`
	Image        *string  `json:"image,omitempty"`
	ComparePrice float64  `json:"comparePrice"`
}

type Product struct {
	ID int `json:"id"`
	ProductFields
}

type ProductInput struct {
	ProductFields
}

type ListProductsResponse struct {
	Items  []Product `json:"items"`
	Total  int       `json:"total"`
	Limit  *int      `json:"limit,omitempty"`
	Offset *int      `json:"offset,omitempty"`
}

func NewService(db database.DB) *Service {
	return &Service{db: db}
}

func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /products", s.create)
	mux.HandleFunc("GET /products", s.list)
	mux.HandleFunc("GET /products/{id}", s.get)
	mux.HandleFunc("PUT /products/{id}", s.update)
	mux.HandleFunc("DELETE /products/{id}", s.delete)
}

func (s *Service) create(w http.ResponseWriter, r *http.Request) {
	var input ProductInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateProductInput(input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	res, err := s.db.Execute(r.Context(), `
		INSERT INTO products (name, price, brand, description, category, image, compare_price)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, input.Name, input.Price, input.Brand, input.Description, input.Category, input.Image, input.ComparePrice)
	if err != nil {
		log.Printf("insert product: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create product")
		return
	}

	writeJSON(w, http.StatusCreated, productFromInput(int(res.LastInsertID), input))
}

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	countRows, err := s.db.Query(r.Context(), `SELECT COUNT(*) AS total FROM products`)
	if err != nil {
		log.Printf("count products: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list products")
		return
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

	rows, err := s.db.Query(r.Context(), query, queryArgs...)
	if err != nil {
		log.Printf("list products: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list products")
		return
	}

	var limitPtr, offsetPtr *int
	if limit > 0 {
		limitPtr = &limit
		offsetPtr = &offset
	}

	items := make([]Product, 0, len(rows))
	for _, row := range rows {
		items = append(items, rowToProduct(row))
	}
	writeJSON(w, http.StatusOK, ListProductsResponse{
		Items:  items,
		Total:  total,
		Limit:  limitPtr,
		Offset: offsetPtr,
	})
}

func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	rows, err := s.db.Query(r.Context(), `
		SELECT id, name, price, brand, description, category, image, compare_price
		FROM products WHERE id = ?
	`, id)
	if err != nil {
		log.Printf("get product %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to get product")
		return
	}

	if len(rows) == 0 {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}

	writeJSON(w, http.StatusOK, rowToProduct(rows[0]))
}

func (s *Service) update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	var input ProductInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateProductInput(input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	res, err := s.db.Execute(r.Context(), `
		UPDATE products
		SET name = ?, price = ?, brand = ?, description = ?, category = ?, image = ?, compare_price = ?
		WHERE id = ?
	`, input.Name, input.Price, input.Brand, input.Description, input.Category, input.Image, input.ComparePrice, id)
	if err != nil {
		log.Printf("update product %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to update product")
		return
	}

	if res.RowsAffected == 0 {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}

	writeJSON(w, http.StatusOK, productFromInput(id, input))
}

func (s *Service) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	res, err := s.db.Execute(r.Context(), `DELETE FROM products WHERE id = ?`, id)
	if err != nil {
		log.Printf("delete product %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to delete product")
		return
	}
	if res.RowsAffected == 0 {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func productFromInput(id int, b ProductInput) Product {
	return Product{ID: id, ProductFields: b.ProductFields}
}

func rowToProduct(row database.Row) Product {
	id, _ := row.Int("id")
	name, _ := row.String("name")
	price, _ := row.Float("price")
	brand, _ := row.NullableString("brand")
	description, _ := row.NullableString("description")
	category, _ := row.String("category")
	image, _ := row.NullableString("image")
	comparePrice, _ := row.Float("compare_price")
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
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func validateProductInput(p ProductInput) error {
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}
	if p.Price < 0 {
		return fmt.Errorf("price must be >= 0")
	}
	if p.Category == "" {
		return fmt.Errorf("category is required")
	}
	return nil
}
