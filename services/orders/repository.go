package orders

import (
	"context"
	"fmt"

	"github.com/thaletto/krcrackers-go/database"
)

type Repository interface {
	Create(ctx context.Context, input OrderInput) (Order, error)
	List(ctx context.Context, limit, offset int) (ListOrdersResponse, error)
	Get(ctx context.Context, id int) (Order, error)
	Update(ctx context.Context, id int, input OrderInput) (Order, error)
	Delete(ctx context.Context, id int) error
}

type repo struct {
	db database.DB
}

func NewRepository(db database.DB) Repository {
	return &repo{db: db}
}

func (r *repo) Create(ctx context.Context, input OrderInput) (Order, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin tx: %w", err)
	}

	res, err := tx.Execute(ctx, `
		INSERT INTO orders (user_name, email, phone, street, town_or_city, state, pincode, notes, delivery_region, delivery_location, total)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, input.UserName, input.Email, input.Phone, input.Street, input.TownOrCity, input.State, input.Pincode, input.Notes, input.DeliveryRegion, input.DeliveryLocation, input.Total)
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
		SELECT id, user_name, email, phone, street, town_or_city, state, pincode, notes, delivery_region, delivery_location, total
		FROM orders
		ORDER BY id
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
	return ListOrdersResponse{
		Items:  items,
		Total:  total,
		Limit:  limitPtr,
		Offset: offsetPtr,
	}, nil
}

func (r *repo) Get(ctx context.Context, id int) (Order, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_name, email, phone, street, town_or_city, state, pincode, notes, delivery_region, delivery_location, total
		FROM orders WHERE id = ?
	`, id)
	if err != nil {
		return Order{}, fmt.Errorf("get order %d: %w", id, err)
	}
	if len(rows) == 0 {
		return Order{}, fmt.Errorf("order not found")
	}

	order, err := rowToOrder(rows[0])
	if err != nil {
		return Order{}, err
	}

	itemRows, err := r.db.Query(ctx, `
		SELECT id, product_id, product_name, price, quantity, total
		FROM order_items WHERE order_id = ?
		ORDER BY id
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
		return Order{}, fmt.Errorf("order not found")
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
		return fmt.Errorf("order not found")
	}

	return tx.Commit()
}

func rowToOrder(row database.Row) (Order, error) {
	id, err := row.Int("id")
	if err != nil {
		return Order{}, err
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
		return Order{}, err
	}
	deliveryLocation, err := row.String("delivery_location")
	if err != nil {
		return Order{}, err
	}
	total, err := row.Float("total")
	if err != nil {
		return Order{}, err
	}
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
