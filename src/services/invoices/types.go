// Package invoices provides on-demand PDF invoice generation for orders.
// Invoice numbers are derived from the order ID (INV-0001, INV-0002, ...)
// and stored on the order record after first generation.
//
// The business Service is HTTP-agnostic; the apis/invoices package adapts
// it to routes.
package invoices

import (
	"context"
	"time"

	"github.com/thaletto/krcrackers-go/src/services/orders"
)

// InvoiceItem represents a line item on an invoice.
type InvoiceItem struct {
	ProductName string  `json:"productName"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unitPrice"`
	LineTotal   float64 `json:"lineTotal"`
}

// Invoice contains all data needed to generate a PDF invoice.
type Invoice struct {
	Number        string        `json:"number"`
	Date          time.Time     `json:"date"`
	OrderID       int           `json:"orderId"`
	CustomerName  string        `json:"customerName"`
	CustomerEmail string        `json:"customerEmail"`
	CustomerPhone string        `json:"customerPhone"`
	Street        string        `json:"street"`
	City          string        `json:"city"`
	State         string        `json:"state"`
	Pincode       string        `json:"pincode"`
	Items         []InvoiceItem `json:"items"`
	Subtotal      float64       `json:"subtotal"`
	Total         float64       `json:"total"`
}

// OrderSource is the seam onto the Order module. The Invoice module never
// queries the orders table: it asks for an order already resolved against
// the calling actor, and asks the Order module to record the number it
// mints. Ownership is therefore decided in one place.
type OrderSource interface {
	InvoiceView(ctx context.Context, actor orders.Actor, orderID int) (orders.InvoiceView, error)
	AssignInvoiceNumber(ctx context.Context, actor orders.Actor, orderID int, number string) error
}

// render maps an order snapshot onto the invoice the PDF generator draws.
// The naming changes are the only translation; no field is invented here.
func render(view orders.InvoiceView, number string, date time.Time) Invoice {
	inv := Invoice{
		Number:        number,
		Date:          date,
		OrderID:       view.OrderID,
		CustomerName:  view.UserName,
		CustomerEmail: view.Email,
		CustomerPhone: view.Phone,
		Street:        view.Street,
		City:          view.TownOrCity,
		State:         view.State,
		Pincode:       view.Pincode,
		Total:         view.Total,
		Items:         make([]InvoiceItem, 0, len(view.Items)),
	}
	for _, line := range view.Items {
		inv.Items = append(inv.Items, InvoiceItem{
			ProductName: line.ProductName,
			Quantity:    line.Quantity,
			UnitPrice:   line.Price,
			LineTotal:   line.Total,
		})
	}
	inv.Subtotal = view.Total
	return inv
}
