// Package invoices provides on-demand PDF invoice generation for orders.
// Invoice numbers are derived from the order ID (INV-0001, INV-0002, ...)
// and stored on the order record after first generation.
//
// The business Service is HTTP-agnostic; the apis/invoices package adapts
// it to routes.
package invoices

import (
	"context"
	"fmt"
	"time"
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

// Repository defines the data access interface for invoice data.
type Repository interface {
	GetOrderWithItems(ctx context.Context, orderID int) (Invoice, error)
	SaveInvoiceNumber(ctx context.Context, orderID int, invoiceNumber string) error
}

// ErrOrderNotFound is returned by Service.GeneratePDF when the order
// does not exist.
var ErrOrderNotFound = fmt.Errorf("order not found")
