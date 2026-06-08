package orders

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

type OrderItemFields struct {
	ProductID   int     `json:"productId"`
	ProductName string  `json:"productName"`
	Price       float64 `json:"price"`
	Quantity    int     `json:"quantity"`
	Total       float64 `json:"total"`
}

type OrderItem struct {
	ID int `json:"id"`
	OrderItemFields
}

type OrderFields struct {
	UserName         string   `json:"userName"`
	Email            string   `json:"email"`
	Phone            string   `json:"phone"`
	Street           string   `json:"street"`
	TownOrCity       string   `json:"townOrCity"`
	State            string   `json:"state"`
	Pincode          string   `json:"pincode"`
	Notes            *string  `json:"notes,omitempty"`
	DeliveryRegion   string   `json:"deliveryRegion"`
	DeliveryLocation string   `json:"deliveryLocation"`
	Total            float64  `json:"total"`
}

type Order struct {
	ID int `json:"id"`
	OrderFields
	Items []OrderItem `json:"items"`
}

type OrderInput struct {
	OrderFields
	Items []OrderItemFields `json:"items"`
}

type ListOrdersResponse struct {
	Items  []Order `json:"items"`
	Total  int     `json:"total"`
	Limit  *int    `json:"limit,omitempty"`
	Offset *int    `json:"offset,omitempty"`
}

func NewService(db database.DB) *Service {
	return &Service{db: db}
}

func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /orders", s.create)
	mux.HandleFunc("GET /orders", s.list)
	mux.HandleFunc("GET /orders/{id}", s.get)
	mux.HandleFunc("PUT /orders/{id}", s.update)
	mux.HandleFunc("DELETE /orders/{id}", s.delete)
}

func (s *Service) create(w http.ResponseWriter, r *http.Request) {
	var input OrderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateOrderInput(input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	res, err := s.db.Execute(r.Context(), `
		INSERT INTO orders (user_name, email, phone, street, town_or_city, state, pincode, notes, delivery_region, delivery_location, total)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, input.UserName, input.Email, input.Phone, input.Street, input.TownOrCity, input.State, input.Pincode, input.Notes, input.DeliveryRegion, input.DeliveryLocation, input.Total)
	if err != nil {
		log.Printf("insert order: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create order")
		return
	}
	orderID := int(res.LastInsertID)

	items := make([]OrderItem, 0, len(input.Items))
	for i, item := range input.Items {
		itemRes, err := s.db.Execute(r.Context(), `
			INSERT INTO order_items (order_id, product_id, product_name, price, quantity, total)
			VALUES (?, ?, ?, ?, ?, ?)
		`, orderID, item.ProductID, item.ProductName, item.Price, item.Quantity, item.Total)
		if err != nil {
			log.Printf("insert order item: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to create order item")
			return
		}
		items = append(items, OrderItem{
			ID:              int(itemRes.LastInsertID),
			OrderItemFields: input.Items[i],
		})
	}

	writeJSON(w, http.StatusCreated, Order{ID: orderID, OrderFields: input.OrderFields, Items: items})
}

func (s *Service) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	countRows, err := s.db.Query(r.Context(), `SELECT COUNT(*) AS total FROM orders`)
	if err != nil {
		log.Printf("count orders: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list orders")
		return
	}
	total := 0
	if len(countRows) > 0 {
		if v, err := countRows[0].Int("total"); err == nil {
			total = int(v)
		}
	}

	query := `
		SELECT id, user_name, email, phone, street, town_or_city, state, pincode, notes, delivery_region, delivery_location, total
		FROM orders
		ORDER BY id
	`
	var queryArgs []any
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		queryArgs = append(queryArgs, limit, offset)
	}

	rows, err := s.db.Query(r.Context(), query, queryArgs...)
	if err != nil {
		log.Printf("list orders: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list orders")
		return
	}

	var limitPtr, offsetPtr *int
	if limit > 0 {
		limitPtr = &limit
		offsetPtr = &offset
	}

	items := make([]Order, 0, len(rows))
	for _, row := range rows {
		items = append(items, rowToOrder(row))
	}
	writeJSON(w, http.StatusOK, ListOrdersResponse{
		Items:  items,
		Total:  total,
		Limit:  limitPtr,
		Offset: offsetPtr,
	})
}

func (s *Service) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	rows, err := s.db.Query(r.Context(), `
		SELECT id, user_name, email, phone, street, town_or_city, state, pincode, notes, delivery_region, delivery_location, total
		FROM orders WHERE id = ?
	`, id)
	if err != nil {
		log.Printf("get order %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to get order")
		return
	}

	if len(rows) == 0 {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}

	order := rowToOrder(rows[0])

	itemRows, err := s.db.Query(r.Context(), `
		SELECT id, product_id, product_name, price, quantity, total
		FROM order_items WHERE order_id = ?
		ORDER BY id
	`, id)
	if err != nil {
		log.Printf("get order items %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to get order items")
		return
	}

	order.Items = make([]OrderItem, 0, len(itemRows))
	for _, row := range itemRows {
		order.Items = append(order.Items, rowToOrderItem(row))
	}

	writeJSON(w, http.StatusOK, order)
}

func (s *Service) update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	var input OrderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateOrderInput(input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	res, err := s.db.Execute(r.Context(), `
		UPDATE orders
		SET user_name = ?, email = ?, phone = ?, street = ?, town_or_city = ?, state = ?, pincode = ?, notes = ?, delivery_region = ?, delivery_location = ?, total = ?
		WHERE id = ?
	`, input.UserName, input.Email, input.Phone, input.Street, input.TownOrCity, input.State, input.Pincode, input.Notes, input.DeliveryRegion, input.DeliveryLocation, input.Total, id)
	if err != nil {
		log.Printf("update order %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to update order")
		return
	}

	if res.RowsAffected == 0 {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}

	_, err = s.db.Execute(r.Context(), `DELETE FROM order_items WHERE order_id = ?`, id)
	if err != nil {
		log.Printf("delete order items %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to update order items")
		return
	}

	items := make([]OrderItem, 0, len(input.Items))
	for i, item := range input.Items {
		itemRes, err := s.db.Execute(r.Context(), `
			INSERT INTO order_items (order_id, product_id, product_name, price, quantity, total)
			VALUES (?, ?, ?, ?, ?, ?)
		`, id, item.ProductID, item.ProductName, item.Price, item.Quantity, item.Total)
		if err != nil {
			log.Printf("insert order item: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to update order items")
			return
		}
		items = append(items, OrderItem{
			ID:              int(itemRes.LastInsertID),
			OrderItemFields: input.Items[i],
		})
	}

	writeJSON(w, http.StatusOK, Order{ID: id, OrderFields: input.OrderFields, Items: items})
}

func (s *Service) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid order id")
		return
	}

	_, err = s.db.Execute(r.Context(), `DELETE FROM order_items WHERE order_id = ?`, id)
	if err != nil {
		log.Printf("delete order items %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to delete order")
		return
	}

	res, err := s.db.Execute(r.Context(), `DELETE FROM orders WHERE id = ?`, id)
	if err != nil {
		log.Printf("delete order %d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to delete order")
		return
	}
	if res.RowsAffected == 0 {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func rowToOrder(row database.Row) Order {
	id, _ := row.Int("id")
	userName, _ := row.String("user_name")
	email, _ := row.String("email")
	phone, _ := row.String("phone")
	street, _ := row.String("street")
	townOrCity, _ := row.String("town_or_city")
	state, _ := row.String("state")
	pincode, _ := row.String("pincode")
	notes, _ := row.NullableString("notes")
	deliveryRegion, _ := row.String("delivery_region")
	deliveryLocation, _ := row.String("delivery_location")
	total, _ := row.Float("total")
	return Order{
		ID: int(id),
		OrderFields: OrderFields{
			UserName:         userName,
			Email:            email,
			Phone:            phone,
			Street:           street,
			TownOrCity:       townOrCity,
			State:            state,
			Pincode:          pincode,
			Notes:            notes,
			DeliveryRegion:   deliveryRegion,
			DeliveryLocation: deliveryLocation,
			Total:            total,
		},
	}
}

func rowToOrderItem(row database.Row) OrderItem {
	id, _ := row.Int("id")
	productID, _ := row.Int("product_id")
	productName, _ := row.String("product_name")
	price, _ := row.Float("price")
	quantity, _ := row.Int("quantity")
	total, _ := row.Float("total")
	return OrderItem{
		ID: int(id),
		OrderItemFields: OrderItemFields{
			ProductID:   int(productID),
			ProductName: productName,
			Price:       price,
			Quantity:    int(quantity),
			Total:       total,
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

func validateOrderInput(o OrderInput) error {
	if o.UserName == "" {
		return fmt.Errorf("userName is required")
	}
	if o.Email == "" {
		return fmt.Errorf("email is required")
	}
	if o.Phone == "" {
		return fmt.Errorf("phone is required")
	}
	if o.Street == "" {
		return fmt.Errorf("street is required")
	}
	if o.TownOrCity == "" {
		return fmt.Errorf("townOrCity is required")
	}
	if o.State == "" {
		return fmt.Errorf("state is required")
	}
	if o.Pincode == "" {
		return fmt.Errorf("pincode is required")
	}
	if o.DeliveryRegion == "" {
		return fmt.Errorf("deliveryRegion is required")
	}
	if o.DeliveryLocation == "" {
		return fmt.Errorf("deliveryLocation is required")
	}
	if len(o.Items) == 0 {
		return fmt.Errorf("at least one item is required")
	}
	return nil
}
