package orders

import (
	"context"
	"fmt"
	"io"

	apperrors "github.com/thaletto/krcrackers-go/errors"
	"github.com/thaletto/krcrackers-go/database"
)

// OrderStatus represents the lifecycle state of an order.
type OrderStatus string

// Valid order status values and their allowed transitions:
//
//	pending -> confirmed, cancelled
//	confirmed -> shipped, cancelled
//	shipped -> delivered, cancelled
//	delivered (terminal)
//	cancelled (terminal)
const (
	StatusPending   OrderStatus = "pending"
	StatusConfirmed OrderStatus = "confirmed"
	StatusShipped   OrderStatus = "shipped"
	StatusDelivered OrderStatus = "delivered"
	StatusCancelled OrderStatus = "cancelled"
)

// OrderItemFields contains the data fields for an order line item.
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

// OrderFields contains the data fields for an order header.
type OrderFields struct {
	Status               OrderStatus `json:"status"`
	UserID               *int        `json:"userId,omitempty"`
	UserName             string      `json:"userName"`
	Email                string      `json:"email"`
	Phone                string      `json:"phone"`
	Street               string      `json:"street"`
	TownOrCity           string      `json:"townOrCity"`
	State                string      `json:"state"`
	Pincode              string      `json:"pincode"`
	Notes                *string     `json:"notes,omitempty"`
	DeliveryRegion       string      `json:"deliveryRegion"`
	DeliveryLocation     string      `json:"deliveryLocation"`
	Total                float64     `json:"total"`
	PaymentScreenshotURL string      `json:"paymentScreenshotUrl,omitempty"`
	PaymentReference     string      `json:"paymentReference,omitempty"`
}

// Order represents a complete order with its line items.
type Order struct {
	ID        int         `json:"id"`
	OrderFields
	Items     []OrderItem `json:"items"`
	CreatedAt string      `json:"createdAt,omitempty"`
}

// OrderInput is the request payload for creating or updating an order.
type OrderInput struct {
	OrderFields
	Items []OrderItemFields `json:"items"`
}

// ListOrdersResponse is the paginated response for order list endpoints.
type ListOrdersResponse struct {
	Items  []Order `json:"items"`
	Total  int     `json:"total"`
	Limit  *int    `json:"limit,omitempty"`
	Offset *int    `json:"offset,omitempty"`
}

// DashboardStats contains aggregated order statistics for the admin dashboard.
type DashboardStats struct {
	TotalOrders    int     `json:"totalOrders"`
	PendingOrders  int     `json:"pendingOrders"`
	RevenueMonth   float64 `json:"revenueMonth"`
	NewCustomers   int     `json:"newCustomers"`
}

// UserProvider is an interface for fetching user data, used during checkout.
type UserProvider interface {
	GetUser(ctx context.Context, id int) (User, error)
}

// AddressProvider is an interface for fetching customer addresses, used during checkout.
type AddressProvider interface {
	GetAddress(ctx context.Context, id int) (Address, error)
}

// UploadsService is an interface for file uploads, used for payment screenshots.
type UploadsService interface {
	Put(ctx context.Context, key string, body io.Reader, contentType string) (string, error)
}

// User holds minimal user data needed for order creation.
type User struct {
	ID    int
	Email string
	Name  string
	Phone string
}

// Address holds shipping address data snapshotted at checkout time.
type Address struct {
	ID      int
	UserID  int
	Label   string
	Street  string
	City    string
	State   string
	Pincode string
	Country string
}

// Repository defines the data access interface for orders.
type Repository interface {
	Create(ctx context.Context, input OrderInput) (Order, error)
	List(ctx context.Context, limit, offset int) (ListOrdersResponse, error)
	Get(ctx context.Context, id int) (Order, error)
	Update(ctx context.Context, id int, input OrderInput) (Order, error)
	Delete(ctx context.Context, id int) error
	Checkout(ctx context.Context, input OrderInput) (Order, error)
	ListForUser(ctx context.Context, userID int, limit, offset int) (ListOrdersResponse, error)
	GetForUser(ctx context.Context, orderID, userID int) (Order, error)
	UpdateStatus(ctx context.Context, orderID int, status OrderStatus) (Order, error)
	ListAllFilter(ctx context.Context, status string, limit, offset int) (ListOrdersResponse, error)
	GetDashboardStats(ctx context.Context) (DashboardStats, error)
}

type repo struct {
	db database.DB
}

// NewRepository returns a new orders repository backed by the given database.
func NewRepository(db database.DB) Repository {
	return &repo{db: db}
}

func (r *repo) Create(ctx context.Context, input OrderInput) (Order, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin tx: %w", err)
	}

	status := input.Status
	if status == "" {
		status = StatusPending
	}

	res, err := tx.Execute(ctx, `
		INSERT INTO orders (user_name, email, phone, street, town_or_city, state, pincode, notes, delivery_region, delivery_location, total, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, input.UserName, input.Email, input.Phone, input.Street, input.TownOrCity, input.State, input.Pincode, input.Notes, input.DeliveryRegion, input.DeliveryLocation, input.Total, string(status))
	if err != nil {
		_ = tx.Rollback()
		return Order{}, fmt.Errorf("insert order: %w", err)
	}
	orderID := int(res.LastInsertID)

	items := make([]OrderItem, 0, len(input.Items))
	for i, item := range input.Items {
		itemRes, err := tx.Execute(ctx, `
			INSERT INTO order_items (order_id, product_id, product_name, price, quantity, total)
			VALUES (?, ?, ?, ?, ?, ?)
		`, orderID, item.ProductID, item.ProductName, item.Price, item.Quantity, item.Total)
		if err != nil {
			_ = tx.Rollback()
			return Order{}, fmt.Errorf("insert order item: %w", err)
		}
		items = append(items, OrderItem{
			ID:              int(itemRes.LastInsertID),
			OrderItemFields: input.Items[i],
		})
	}

	if err := tx.Commit(); err != nil {
		return Order{}, fmt.Errorf("commit tx: %w", err)
	}

	return Order{ID: orderID, OrderFields: input.OrderFields, Items: items}, nil
}

func (r *repo) List(ctx context.Context, limit, offset int) (ListOrdersResponse, error) {
	countRows, err := r.db.Query(ctx, `SELECT COUNT(*) AS total FROM orders`)
	if err != nil {
		return ListOrdersResponse{}, fmt.Errorf("count orders: %w", err)
	}
	total := 0
	if len(countRows) > 0 {
		if v, err := countRows[0].Int("total"); err == nil {
			total = int(v)
		}
	}

	query := `
		SELECT id, status, user_id, user_name, email, phone, street, town_or_city, state, pincode, notes, delivery_region, delivery_location, total, payment_screenshot_url, payment_reference, created_at
		FROM orders ORDER BY id DESC
	`
	var queryArgs []any
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		queryArgs = append(queryArgs, limit, offset)
	}

	rows, err := r.db.Query(ctx, query, queryArgs...)
	if err != nil {
		return ListOrdersResponse{}, fmt.Errorf("list orders: %w", err)
	}

	var limitPtr, offsetPtr *int
	if limit > 0 {
		limitPtr = &limit
		offsetPtr = &offset
	}

	items := make([]Order, 0, len(rows))
	for _, row := range rows {
		o, err := rowToOrder(row)
		if err != nil {
			return ListOrdersResponse{}, fmt.Errorf("scan order: %w", err)
		}
		items = append(items, o)
	}
	return ListOrdersResponse{Items: items, Total: total, Limit: limitPtr, Offset: offsetPtr}, nil
}

func (r *repo) Get(ctx context.Context, id int) (Order, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, status, user_id, user_name, email, phone, street, town_or_city, state, pincode, notes, delivery_region, delivery_location, total, payment_screenshot_url, payment_reference, created_at
		FROM orders WHERE id = ?
	`, id)
	if err != nil {
		return Order{}, fmt.Errorf("get order %d: %w", id, err)
	}
	if len(rows) == 0 {
		return Order{}, fmt.Errorf("order %d: %w", id, apperrors.ErrNotFound)
	}

	order, err := rowToOrder(rows[0])
	if err != nil {
		return Order{}, err
	}

	itemRows, err := r.db.Query(ctx, `
		SELECT id, product_id, product_name, price, quantity, total
		FROM order_items WHERE order_id = ? ORDER BY id
	`, id)
	if err != nil {
		return Order{}, fmt.Errorf("get order items %d: %w", id, err)
	}

	order.Items = make([]OrderItem, 0, len(itemRows))
	for _, row := range itemRows {
		item, err := rowToOrderItem(row)
		if err != nil {
			return Order{}, err
		}
		order.Items = append(order.Items, item)
	}

	return order, nil
}

func (r *repo) Update(ctx context.Context, id int, input OrderInput) (Order, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin tx: %w", err)
	}

	res, err := tx.Execute(ctx, `
		UPDATE orders
		SET user_name = ?, email = ?, phone = ?, street = ?, town_or_city = ?, state = ?, pincode = ?, notes = ?, delivery_region = ?, delivery_location = ?, total = ?
		WHERE id = ?
	`, input.UserName, input.Email, input.Phone, input.Street, input.TownOrCity, input.State, input.Pincode, input.Notes, input.DeliveryRegion, input.DeliveryLocation, input.Total, id)
	if err != nil {
		_ = tx.Rollback()
		return Order{}, fmt.Errorf("update order %d: %w", id, err)
	}
	if res.RowsAffected == 0 {
		_ = tx.Rollback()
		return Order{}, fmt.Errorf("order %d: %w", id, apperrors.ErrNotFound)
	}

	_, err = tx.Execute(ctx, `DELETE FROM order_items WHERE order_id = ?`, id)
	if err != nil {
		_ = tx.Rollback()
		return Order{}, fmt.Errorf("delete order items %d: %w", id, err)
	}

	items := make([]OrderItem, 0, len(input.Items))
	for i, item := range input.Items {
		itemRes, err := tx.Execute(ctx, `
			INSERT INTO order_items (order_id, product_id, product_name, price, quantity, total)
			VALUES (?, ?, ?, ?, ?, ?)
		`, id, item.ProductID, item.ProductName, item.Price, item.Quantity, item.Total)
		if err != nil {
			_ = tx.Rollback()
			return Order{}, fmt.Errorf("insert order item: %w", err)
		}
		items = append(items, OrderItem{
			ID:              int(itemRes.LastInsertID),
			OrderItemFields: input.Items[i],
		})
	}

	if err := tx.Commit(); err != nil {
		return Order{}, fmt.Errorf("commit tx: %w", err)
	}

	return Order{ID: id, OrderFields: input.OrderFields, Items: items}, nil
}

func (r *repo) Delete(ctx context.Context, id int) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	_, err = tx.Execute(ctx, `DELETE FROM order_items WHERE order_id = ?`, id)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete order items %d: %w", id, err)
	}

	res, err := tx.Execute(ctx, `DELETE FROM orders WHERE id = ?`, id)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete order %d: %w", id, err)
	}
	if res.RowsAffected == 0 {
		_ = tx.Rollback()
		return fmt.Errorf("order %d: %w", id, apperrors.ErrNotFound)
	}

	return tx.Commit()
}

func (r *repo) Checkout(ctx context.Context, input OrderInput) (Order, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin tx: %w", err)
	}

	status := input.Status
	if status == "" {
		status = StatusPending
	}

	res, err := tx.Execute(ctx, `
		INSERT INTO orders (status, user_id, user_name, email, phone, street, town_or_city, state, pincode, notes, delivery_region, delivery_location, total, payment_screenshot_url, payment_reference)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, string(status), input.UserID, input.UserName, input.Email, input.Phone, input.Street, input.TownOrCity, input.State, input.Pincode, input.Notes, input.DeliveryRegion, input.DeliveryLocation, input.Total, input.PaymentScreenshotURL, input.PaymentReference)
	if err != nil {
		_ = tx.Rollback()
		return Order{}, fmt.Errorf("insert order: %w", err)
	}
	orderID := int(res.LastInsertID)

	items := make([]OrderItem, 0, len(input.Items))
	for i, item := range input.Items {
		itemRes, err := tx.Execute(ctx, `
			INSERT INTO order_items (order_id, product_id, product_name, price, quantity, total)
			VALUES (?, ?, ?, ?, ?, ?)
		`, orderID, item.ProductID, item.ProductName, item.Price, item.Quantity, item.Total)
		if err != nil {
			_ = tx.Rollback()
			return Order{}, fmt.Errorf("insert order item: %w", err)
		}
		items = append(items, OrderItem{
			ID:              int(itemRes.LastInsertID),
			OrderItemFields: input.Items[i],
		})
	}

	if err := tx.Commit(); err != nil {
		return Order{}, fmt.Errorf("commit tx: %w", err)
	}

	return Order{ID: orderID, OrderFields: input.OrderFields, Items: items}, nil
}

func (r *repo) ListForUser(ctx context.Context, userID int, limit, offset int) (ListOrdersResponse, error) {
	countRows, err := r.db.Query(ctx, `SELECT COUNT(*) AS total FROM orders WHERE user_id = ?`, userID)
	if err != nil {
		return ListOrdersResponse{}, err
	}
	total := 0
	if len(countRows) > 0 {
		if v, err := countRows[0].Int("total"); err == nil {
			total = int(v)
		}
	}

	query := `
		SELECT id, status, user_id, user_name, email, phone, street, town_or_city, state, pincode, notes, delivery_region, delivery_location, total, payment_screenshot_url, payment_reference, created_at
		FROM orders WHERE user_id = ? ORDER BY id DESC
	`
	var args []any
	args = append(args, userID)
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return ListOrdersResponse{}, err
	}

	var limitPtr, offsetPtr *int
	if limit > 0 {
		limitPtr = &limit
		offsetPtr = &offset
	}

	orders := make([]Order, 0, len(rows))
	for _, row := range rows {
		o, err := rowToOrder(row)
		if err != nil {
			return ListOrdersResponse{}, err
		}
		orders = append(orders, o)
	}
	return ListOrdersResponse{Items: orders, Total: total, Limit: limitPtr, Offset: offsetPtr}, nil
}

func (r *repo) GetForUser(ctx context.Context, orderID, userID int) (Order, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, status, user_id, user_name, email, phone, street, town_or_city, state, pincode, notes, delivery_region, delivery_location, total, payment_screenshot_url, payment_reference, created_at
		FROM orders WHERE id = ? AND user_id = ?
	`, orderID, userID)
	if err != nil {
		return Order{}, err
	}
	if len(rows) == 0 {
		return Order{}, fmt.Errorf("order %d: %w", orderID, apperrors.ErrNotFound)
	}

	order, err := rowToOrder(rows[0])
	if err != nil {
		return Order{}, err
	}

	itemRows, err := r.db.Query(ctx, `
		SELECT id, product_id, product_name, price, quantity, total
		FROM order_items WHERE order_id = ? ORDER BY id
	`, orderID)
	if err != nil {
		return Order{}, err
	}

	order.Items = make([]OrderItem, 0, len(itemRows))
	for _, row := range itemRows {
		item, err := rowToOrderItem(row)
		if err != nil {
			return Order{}, err
		}
		order.Items = append(order.Items, item)
	}

	return order, nil
}

func (r *repo) UpdateStatus(ctx context.Context, orderID int, status OrderStatus) (Order, error) {
	order, err := r.Get(ctx, orderID)
	if err != nil {
		return Order{}, err
	}

	if !isValidTransition(order.Status, status) {
		return Order{}, fmt.Errorf("invalid status transition from %s to %s", order.Status, status)
	}

	_, err = r.db.Execute(ctx, `UPDATE orders SET status = ? WHERE id = ?`, string(status), orderID)
	if err != nil {
		return Order{}, err
	}

	if status == StatusConfirmed {
		r.db.Execute(ctx, `UPDATE orders SET verified_at = CURRENT_TIMESTAMP WHERE id = ?`, orderID)
	}

	return r.Get(ctx, orderID)
}

func (r *repo) ListAllFilter(ctx context.Context, status string, limit, offset int) (ListOrdersResponse, error) {
	where := "1=1"
	var args []any
	if status != "" {
		where = "status = ?"
		args = append(args, status)
	}

	countRows, err := r.db.Query(ctx, `SELECT COUNT(*) AS total FROM orders WHERE `+where, args...)
	if err != nil {
		return ListOrdersResponse{}, err
	}
	total := 0
	if len(countRows) > 0 {
		if v, err := countRows[0].Int("total"); err == nil {
			total = int(v)
		}
	}

	query := `
		SELECT id, status, user_id, user_name, email, phone, street, town_or_city, state, pincode, notes, delivery_region, delivery_location, total, payment_screenshot_url, payment_reference, created_at
		FROM orders WHERE ` + where + ` ORDER BY id DESC
	`
	var queryArgs = make([]any, len(args))
	copy(queryArgs, args)
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		queryArgs = append(queryArgs, limit, offset)
	}

	rows, err := r.db.Query(ctx, query, queryArgs...)
	if err != nil {
		return ListOrdersResponse{}, err
	}

	var limitPtr, offsetPtr *int
	if limit > 0 {
		limitPtr = &limit
		offsetPtr = &offset
	}

	orders := make([]Order, 0, len(rows))
	for _, row := range rows {
		o, err := rowToOrder(row)
		if err != nil {
			return ListOrdersResponse{}, err
		}
		orders = append(orders, o)
	}
	return ListOrdersResponse{Items: orders, Total: total, Limit: limitPtr, Offset: offsetPtr}, nil
}

func (r *repo) GetDashboardStats(ctx context.Context) (DashboardStats, error) {
	var stats DashboardStats

	rows, err := r.db.Query(ctx, `SELECT COUNT(*) AS total FROM orders`)
	if err == nil && len(rows) > 0 {
		if v, err := rows[0].Int("total"); err == nil {
			stats.TotalOrders = int(v)
		}
	}

	rows, err = r.db.Query(ctx, `SELECT COUNT(*) AS total FROM orders WHERE status = 'pending'`)
	if err == nil && len(rows) > 0 {
		if v, err := rows[0].Int("total"); err == nil {
			stats.PendingOrders = int(v)
		}
	}

	rows, err = r.db.Query(ctx, `SELECT COALESCE(SUM(total), 0) AS revenue FROM orders WHERE created_at >= date('now', 'start of month')`)
	if err == nil && len(rows) > 0 {
		if v, err := rows[0].Float("revenue"); err == nil {
			stats.RevenueMonth = v
		}
	}

	rows, err = r.db.Query(ctx, `SELECT COUNT(*) AS total FROM users WHERE created_at >= date('now', 'start of month')`)
	if err == nil && len(rows) > 0 {
		if v, err := rows[0].Int("total"); err == nil {
			stats.NewCustomers = int(v)
		}
	}

	return stats, nil
}

var validTransitions = map[OrderStatus][]OrderStatus{
	StatusPending:   {StatusConfirmed, StatusCancelled},
	StatusConfirmed: {StatusShipped, StatusCancelled},
	StatusShipped:   {StatusDelivered, StatusCancelled},
}

func isValidTransition(from, to OrderStatus) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

func rowToOrder(row database.Row) (Order, error) {
	id, err := row.Int("id")
	if err != nil {
		return Order{}, err
	}
	statusStr, err := row.String("status")
	if err != nil {
		statusStr = "pending"
	}
	var userID *int
	if userIDVal, err := row.Int("user_id"); err == nil && userIDVal > 0 {
		uid := int(userIDVal)
		userID = &uid
	}
	userName, err := row.String("user_name")
	if err != nil {
		return Order{}, err
	}
	email, err := row.String("email")
	if err != nil {
		return Order{}, err
	}
	phone, err := row.String("phone")
	if err != nil {
		return Order{}, err
	}
	street, err := row.String("street")
	if err != nil {
		return Order{}, err
	}
	townOrCity, err := row.String("town_or_city")
	if err != nil {
		return Order{}, err
	}
	state, err := row.String("state")
	if err != nil {
		return Order{}, err
	}
	pincode, err := row.String("pincode")
	if err != nil {
		return Order{}, err
	}
	notes, err := row.NullableString("notes")
	if err != nil {
		return Order{}, err
	}
	deliveryRegion, err := row.String("delivery_region")
	if err != nil {
		deliveryRegion = ""
	}
	deliveryLocation, err := row.String("delivery_location")
	if err != nil {
		deliveryLocation = ""
	}
	total, err := row.Float("total")
	if err != nil {
		return Order{}, err
	}
	screenshotURL, err := row.String("payment_screenshot_url")
	if err != nil {
		screenshotURL = ""
	}
	paymentRef, err := row.String("payment_reference")
	if err != nil {
		paymentRef = ""
	}
	createdAt, err := row.String("created_at")
	if err != nil {
		createdAt = ""
	}

	return Order{
		ID: int(id),
		OrderFields: OrderFields{
			Status:               OrderStatus(statusStr),
			UserID:               userID,
			UserName:             userName,
			Email:                email,
			Phone:                phone,
			Street:               street,
			TownOrCity:           townOrCity,
			State:                state,
			Pincode:              pincode,
			Notes:                notes,
			DeliveryRegion:       deliveryRegion,
			DeliveryLocation:     deliveryLocation,
			Total:                total,
			PaymentScreenshotURL: screenshotURL,
			PaymentReference:     paymentRef,
		},
		CreatedAt: createdAt,
	}, nil
}

func rowToOrderItem(row database.Row) (OrderItem, error) {
	id, err := row.Int("id")
	if err != nil {
		return OrderItem{}, err
	}
	productID, err := row.Int("product_id")
	if err != nil {
		return OrderItem{}, err
	}
	productName, err := row.String("product_name")
	if err != nil {
		return OrderItem{}, err
	}
	price, err := row.Float("price")
	if err != nil {
		return OrderItem{}, err
	}
	quantity, err := row.Int("quantity")
	if err != nil {
		return OrderItem{}, err
	}
	total, err := row.Float("total")
	if err != nil {
		return OrderItem{}, err
	}
	return OrderItem{
		ID: int(id),
		OrderItemFields: OrderItemFields{
			ProductID:   int(productID),
			ProductName: productName,
			Price:       price,
			Quantity:    int(quantity),
			Total:       total,
		},
	}, nil
}
