package orders

import (
	"context"
	"fmt"
	"time"

	"github.com/thaletto/krcrackers-go/src/database"
	apperrors "github.com/thaletto/krcrackers-go/src/errors"
)

// orderColumns is the single projection of the orders table. Every read uses
// it so the column list and rowToOrder cannot drift apart; adding a column
// here is a one-line change rather than a search across five queries.
const orderColumns = `id, status, user_id, user_name, email, phone, street, town_or_city, state, pincode, ` +
	`notes, delivery_region, delivery_location, total, payment_screenshot_url, payment_reference, created_at`

// orderItemColumns is the single projection of the order_items table.
const orderItemColumns = `id, product_id, product_name, price, quantity, total`

// Repository defines the data access interface for orders.
//
// Every method that reads or writes a single order takes a Scope, so the
// ownership check is part of the query rather than something a caller
// remembers to apply. There is deliberately no unscoped variant.
type Repository interface {
	// Insert persists a new order header and its items in one transaction.
	Insert(ctx context.Context, input OrderInput) (Order, error)

	// List returns a page of orders visible within the scope, newest first.
	List(ctx context.Context, filter ListFilter, scope Scope) (ListOrdersResponse, error)

	// Get returns one order visible within the scope, or
	// apperrors.ErrNotFound if it does not exist or is out of scope.
	Get(ctx context.Context, id int, scope Scope) (Order, error)

	// Update replaces the mutable fields and the line items of one order
	// within the scope.
	Update(ctx context.Context, id int, input OrderInput, scope Scope) (Order, error)

	// Delete removes one order and its items within the scope.
	Delete(ctx context.Context, id int, scope Scope) error

	// UpdateStatus moves one order to a new status within the scope,
	// refusing a transition the lifecycle does not allow.
	UpdateStatus(ctx context.Context, id int, status OrderStatus, scope Scope) (Order, error)

	// InvoiceView returns the flat order snapshot an invoice renders,
	// resolved against the scope.
	InvoiceView(ctx context.Context, id int, scope Scope) (InvoiceView, error)

	// AssignInvoiceNumber records the invoice number minted for an order
	// within the scope.
	AssignInvoiceNumber(ctx context.Context, id int, number string, scope Scope) error

	// DashboardStats aggregates figures across every order since the given
	// time. It is administrator-only and therefore unscoped by design.
	DashboardStats(ctx context.Context, since time.Time) (DashboardStats, error)
}

type repo struct {
	db database.DB
}

// NewRepository returns a new orders repository backed by the given database.
func NewRepository(db database.DB) Repository {
	return &repo{db: db}
}

func (r *repo) Insert(ctx context.Context, input OrderInput) (Order, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin tx: %w", err)
	}

	status := input.Status
	if status == "" {
		status = StatusPending
	}

	// user_id is nullable: it stays NULL for guest checkout and is what
	// orders.Scope filters on, so an order placed by a signed-in shopper only
	// appears in their history when it is recorded here.
	res, err := tx.Execute(ctx, `
		INSERT INTO orders (status, user_id, user_name, email, phone, street, town_or_city, state, pincode, notes, delivery_region, delivery_location, total, payment_screenshot_url, payment_reference)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, string(status), input.UserID, input.UserName, input.Email, input.Phone, input.Street, input.TownOrCity, input.State, input.Pincode, input.Notes, input.DeliveryRegion, input.DeliveryLocation, input.Total, input.PaymentScreenshotURL, input.PaymentReference)
	if err != nil {
		_ = tx.Rollback()
		return Order{}, fmt.Errorf("insert order: %w", err)
	}
	orderID := int(res.LastInsertID)

	if _, err := insertItems(ctx, tx, orderID, input.Items); err != nil {
		_ = tx.Rollback()
		return Order{}, err
	}

	if err := tx.Commit(); err != nil {
		return Order{}, fmt.Errorf("commit tx: %w", err)
	}

	// Re-read rather than echoing the input: the stored row carries the
	// status default, the timestamps and the item ids the caller cannot know.
	// Insert is reachable only by the module itself, so AnyOrder is correct.
	return r.Get(ctx, orderID, AnyOrder())
}

func (r *repo) List(ctx context.Context, filter ListFilter, scope Scope) (ListOrdersResponse, error) {
	where, args := scope.clause()
	if filter.Status != "" {
		where += " AND status = ?"
		args = append(args, filter.Status)
	}

	query := `SELECT ` + orderColumns + `, COUNT(*) OVER() AS total_count FROM orders WHERE ` + where + ` ORDER BY id DESC`
	if filter.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, filter.Limit, filter.Offset)
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return ListOrdersResponse{}, fmt.Errorf("list orders: %w", err)
	}

	total := 0
	if len(rows) > 0 {
		if v, err := rows[0].Int("total_count"); err == nil {
			total = int(v)
		}
	}

	orders := make([]Order, 0, len(rows))
	for _, row := range rows {
		o, err := rowToOrder(row)
		if err != nil {
			return ListOrdersResponse{}, fmt.Errorf("scan order: %w", err)
		}
		orders = append(orders, o)
	}
	populated, err := r.withItems(ctx, orders)
	if err != nil {
		return ListOrdersResponse{}, err
	}

	var limitPtr, offsetPtr *int
	if filter.Limit > 0 {
		limit := filter.Limit
		offset := filter.Offset
		limitPtr, offsetPtr = &limit, &offset
	}
	return ListOrdersResponse{Items: populated, Total: total, Limit: limitPtr, Offset: offsetPtr}, nil
}

func (r *repo) Get(ctx context.Context, id int, scope Scope) (Order, error) {
	where, args := scope.predicate(id)
	rows, err := r.db.Query(ctx, `SELECT `+orderColumns+` FROM orders WHERE `+where, args...)
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
	order.Items, err = r.itemsFor(ctx, id)
	if err != nil {
		return Order{}, fmt.Errorf("get order items %d: %w", id, err)
	}
	return order, nil
}

func (r *repo) Update(ctx context.Context, id int, input OrderInput, scope Scope) (Order, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin tx: %w", err)
	}

	where, _ := scope.predicate(id)
	res, err := tx.Execute(ctx, `
		UPDATE orders
		SET user_name = ?, email = ?, phone = ?, street = ?, town_or_city = ?, state = ?, pincode = ?, notes = ?, delivery_region = ?, delivery_location = ?, total = ?
		WHERE `+where, scope.writeParams(id,
		input.UserName, input.Email, input.Phone, input.Street, input.TownOrCity,
		input.State, input.Pincode, input.Notes, input.DeliveryRegion,
		input.DeliveryLocation, input.Total)...)
	if err != nil {
		_ = tx.Rollback()
		return Order{}, fmt.Errorf("update order %d: %w", id, err)
	}
	if res.RowsAffected == 0 {
		_ = tx.Rollback()
		return Order{}, fmt.Errorf("order %d: %w", id, apperrors.ErrNotFound)
	}

	if _, err := tx.Execute(ctx, `DELETE FROM order_items WHERE order_id = ?`, id); err != nil {
		_ = tx.Rollback()
		return Order{}, fmt.Errorf("delete order items %d: %w", id, err)
	}

	if _, err := insertItems(ctx, tx, id, input.Items); err != nil {
		_ = tx.Rollback()
		return Order{}, err
	}

	if err := tx.Commit(); err != nil {
		return Order{}, fmt.Errorf("commit tx: %w", err)
	}

	// Re-read for the same reason Insert does: the response should be the row
	// as stored, not the request that produced it.
	return r.Get(ctx, id, scope)
}

func (r *repo) Delete(ctx context.Context, id int, scope Scope) error {
	where, args := scope.predicate(id)

	// Confirm the order is in scope before removing anything. The line items
	// have to go before the header — order_items.order_id is a foreign key
	// with no ON DELETE CASCADE and foreign keys are enforced — so an
	// out-of-scope delete that reached the item delete would destroy another
	// account's order on a backend whose rollback cannot restore a DELETE.
	owned, err := r.db.Query(ctx, `SELECT id FROM orders WHERE `+where, args...)
	if err != nil {
		return fmt.Errorf("find order %d: %w", id, err)
	}
	if len(owned) == 0 {
		return fmt.Errorf("order %d: %w", id, apperrors.ErrNotFound)
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	if _, err := tx.Execute(ctx, `DELETE FROM order_items WHERE order_id = ?`, id); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete order items %d: %w", id, err)
	}

	if _, err := tx.Execute(ctx, `DELETE FROM orders WHERE `+where, args...); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete order %d: %w", id, err)
	}

	return tx.Commit()
}

func (r *repo) UpdateStatus(ctx context.Context, id int, status OrderStatus, scope Scope) (Order, error) {
	where, args := scope.predicate(id)

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin tx: %w", err)
	}

	// Read and write inside one transaction so two concurrent transitions
	// cannot both pass the guard and have the second overwrite the first.
	statusRows, err := tx.Query(ctx, `SELECT status FROM orders WHERE `+where, args...)
	if err != nil {
		_ = tx.Rollback()
		return Order{}, fmt.Errorf("read order status %d: %w", id, err)
	}
	if len(statusRows) == 0 {
		_ = tx.Rollback()
		return Order{}, fmt.Errorf("order %d: %w", id, apperrors.ErrNotFound)
	}
	current, err := statusRows[0].String("status")
	if err != nil {
		_ = tx.Rollback()
		return Order{}, err
	}
	if !isValidTransition(OrderStatus(current), status) {
		_ = tx.Rollback()
		return Order{}, fmt.Errorf("%w: from %s to %s", ErrInvalidStatusTransition, current, status)
	}

	res, err := tx.Execute(ctx, `UPDATE orders SET status = ? WHERE `+where, scope.writeParams(id, string(status))...)
	if err != nil {
		_ = tx.Rollback()
		return Order{}, fmt.Errorf("update order status %d: %w", id, err)
	}
	if res.RowsAffected == 0 {
		_ = tx.Rollback()
		return Order{}, fmt.Errorf("order %d: %w", id, apperrors.ErrNotFound)
	}
	if status == StatusConfirmed {
		if _, err := tx.Execute(ctx, `UPDATE orders SET verified_at = CURRENT_TIMESTAMP WHERE `+where, args...); err != nil {
			_ = tx.Rollback()
			return Order{}, fmt.Errorf("stamp order verified_at %d: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Order{}, fmt.Errorf("commit tx: %w", err)
	}

	// Re-read outside the transaction so the caller sees the stored row
	// rather than a value assembled from the input.
	return r.Get(ctx, id, scope)
}

func (r *repo) InvoiceView(ctx context.Context, id int, scope Scope) (InvoiceView, error) {
	where, args := scope.predicate(id)
	rows, err := r.db.Query(ctx, `
		SELECT id, invoice_number, user_name, email, phone, street, town_or_city, state, pincode, total
		FROM orders WHERE `+where, args...)
	if err != nil {
		return InvoiceView{}, fmt.Errorf("invoice view %d: %w", id, err)
	}
	if len(rows) == 0 {
		return InvoiceView{}, fmt.Errorf("order %d: %w", id, apperrors.ErrNotFound)
	}
	row := rows[0]

	view := InvoiceView{}
	if view.OrderID, err = rowInt(row, "id"); err != nil {
		return InvoiceView{}, err
	}
	if view.Number, err = rowString(row, "invoice_number"); err != nil {
		return InvoiceView{}, err
	}
	if view.UserName, err = rowString(row, "user_name"); err != nil {
		return InvoiceView{}, err
	}
	if view.Email, err = rowString(row, "email"); err != nil {
		return InvoiceView{}, err
	}
	if view.Phone, err = rowString(row, "phone"); err != nil {
		return InvoiceView{}, err
	}
	if view.Street, err = rowString(row, "street"); err != nil {
		return InvoiceView{}, err
	}
	if view.TownOrCity, err = rowString(row, "town_or_city"); err != nil {
		return InvoiceView{}, err
	}
	if view.State, err = rowString(row, "state"); err != nil {
		return InvoiceView{}, err
	}
	if view.Pincode, err = rowString(row, "pincode"); err != nil {
		return InvoiceView{}, err
	}
	if view.Total, err = row.Float("total"); err != nil {
		return InvoiceView{}, err
	}

	itemRows, err := r.itemsFor(ctx, id)
	if err != nil {
		return InvoiceView{}, fmt.Errorf("invoice items %d: %w", id, err)
	}
	view.Items = make([]InvoiceLine, 0, len(itemRows))
	for _, item := range itemRows {
		view.Items = append(view.Items, InvoiceLine{
			ProductName: item.ProductName,
			Quantity:    item.Quantity,
			Price:       item.Price,
			Total:       item.Total,
		})
	}
	return view, nil
}

func (r *repo) AssignInvoiceNumber(ctx context.Context, id int, number string, scope Scope) error {
	where, _ := scope.predicate(id)
	res, err := r.db.Execute(ctx, `UPDATE orders SET invoice_number = ? WHERE `+where, scope.writeParams(id, number)...)
	if err != nil {
		return fmt.Errorf("save invoice number %d: %w", id, err)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("order %d: %w", id, apperrors.ErrNotFound)
	}
	return nil
}

func (r *repo) DashboardStats(ctx context.Context, since time.Time) (DashboardStats, error) {
	var stats DashboardStats

	// One round trip over the orders table, which this module owns. The
	// new-customer figure is the Account module's to count; the caller
	// composes it in rather than this module reaching into users.
	rows, err := r.db.Query(ctx, `
		SELECT
			(SELECT COUNT(*) FROM orders) AS total_orders,
			(SELECT COUNT(*) FROM orders WHERE status = 'pending') AS pending_orders,
			(SELECT COALESCE(SUM(total), 0.0) FROM orders WHERE created_at >= ?) AS revenue_month
	`, since.UTC().Format(time.DateTime))
	if err != nil {
		return stats, fmt.Errorf("dashboard stats: %w", err)
	}
	if len(rows) == 0 {
		return stats, nil
	}
	if v, err := rows[0].Int("total_orders"); err == nil {
		stats.TotalOrders = int(v)
	}
	if v, err := rows[0].Int("pending_orders"); err == nil {
		stats.PendingOrders = int(v)
	}
	if v, err := rows[0].Float("revenue_month"); err == nil {
		stats.RevenueMonth = v
	}

	return stats, nil
}

// insertItems writes every line item of an order inside an open transaction.
func insertItems(ctx context.Context, tx database.Tx, orderID int, fields []OrderItemFields) ([]OrderItem, error) {
	items := make([]OrderItem, 0, len(fields))
	for i, item := range fields {
		itemRes, err := tx.Execute(ctx, `
			INSERT INTO order_items (order_id, product_id, product_name, price, quantity, total)
			VALUES (?, ?, ?, ?, ?, ?)
		`, orderID, item.ProductID, item.ProductName, item.Price, item.Quantity, item.Total)
		if err != nil {
			return nil, fmt.Errorf("insert order item: %w", err)
		}
		items = append(items, OrderItem{
			ID:              int(itemRes.LastInsertID),
			OrderItemFields: fields[i],
		})
	}
	return items, nil
}

// itemsFor loads the line items of one order.
func (r *repo) itemsFor(ctx context.Context, orderID int) ([]OrderItem, error) {
	rows, err := r.db.Query(ctx, `SELECT `+orderItemColumns+` FROM order_items WHERE order_id = ? ORDER BY id`, orderID)
	if err != nil {
		return nil, err
	}
	items := make([]OrderItem, 0, len(rows))
	for _, row := range rows {
		item, err := rowToOrderItem(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// withItems loads order_items for every order in a list response.
// List queries select from orders only, so without this the Items field
// stays empty and callers (e.g. item counts) report zero.
func (r *repo) withItems(ctx context.Context, orders []Order) ([]Order, error) {
	for i, o := range orders {
		items, err := r.itemsFor(ctx, o.ID)
		if err != nil {
			return nil, err
		}
		orders[i].Items = items
	}
	return orders, nil
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
	// user_id is NULL for a guest order, so a read failure and a null read
	// both mean "unowned" rather than an error.
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

func rowInt(row database.Row, col string) (int, error) {
	v, err := row.Int(col)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", col, err)
	}
	return int(v), nil
}

func rowString(row database.Row, col string) (string, error) {
	v, err := row.String(col)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", col, err)
	}
	return v, nil
}
