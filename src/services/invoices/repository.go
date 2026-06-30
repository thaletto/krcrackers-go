package invoices

import (
	"context"

	"github.com/thaletto/krcrackers-go/src/database"
)

type repo struct {
	db database.DB
}

// NewRepository returns a new invoices repository backed by the given database.
func NewRepository(db database.DB) Repository {
	return &repo{db: db}
}

func (r *repo) GetOrderWithItems(ctx context.Context, orderID int) (Invoice, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_name, email, phone, street, town_or_city, state, pincode, total, invoice_number
		FROM orders WHERE id = ?
	`, orderID)
	if err != nil {
		return Invoice{}, err
	}
	if len(rows) == 0 {
		return Invoice{}, ErrOrderNotFound
	}

	orderIDVal, _ := rows[0].Int("id")
	userName, _ := rows[0].String("user_name")
	email, _ := rows[0].String("email")
	phone, _ := rows[0].String("phone")
	street, _ := rows[0].String("street")
	city, _ := rows[0].String("town_or_city")
	state, _ := rows[0].String("state")
	pincode, _ := rows[0].String("pincode")
	total, _ := rows[0].Float("total")
	invoiceNumber, _ := rows[0].String("invoice_number")

	itemRows, err := r.db.Query(ctx, `
		SELECT product_name, quantity, price, total
		FROM order_items WHERE order_id = ? ORDER BY id
	`, orderID)
	if err != nil {
		return Invoice{}, err
	}

	items := make([]InvoiceItem, 0, len(itemRows))
	for _, row := range itemRows {
		productName, _ := row.String("product_name")
		quantity, _ := row.Int("quantity")
		price, _ := row.Float("price")
		lineTotal, _ := row.Float("total")
		items = append(items, InvoiceItem{
			ProductName: productName,
			Quantity:    int(quantity),
			UnitPrice:   price,
			LineTotal:   lineTotal,
		})
	}

	return Invoice{
		Number:        invoiceNumber,
		OrderID:       int(orderIDVal),
		CustomerName:  userName,
		CustomerEmail: email,
		CustomerPhone: phone,
		Street:        street,
		City:          city,
		State:         state,
		Pincode:       pincode,
		Items:         items,
		Subtotal:      total,
		Total:         total,
	}, nil
}

func (r *repo) SaveInvoiceNumber(ctx context.Context, orderID int, invoiceNumber string) error {
	_, err := r.db.Execute(ctx, `UPDATE orders SET invoice_number = ? WHERE id = ?`, invoiceNumber, orderID)
	return err
}
