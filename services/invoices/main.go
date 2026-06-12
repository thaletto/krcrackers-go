// Package invoices provides on-demand PDF invoice generation for orders.
// Invoice numbers are sequential (INV-0001, INV-0002, ...) and stored
// on the order record after first generation.
package invoices

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jung-kurt/gofpdf"
	"github.com/thaletto/krcrackers-go/database"
	"github.com/thaletto/krcrackers-go/server"
	"github.com/thaletto/krcrackers-go/services/auth"
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
	Number         string        `json:"number"`
	Date           time.Time     `json:"date"`
	OrderID        int           `json:"orderId"`
	CustomerName   string        `json:"customerName"`
	CustomerEmail  string        `json:"customerEmail"`
	CustomerPhone  string        `json:"customerPhone"`
	Street         string        `json:"street"`
	City           string        `json:"city"`
	State          string        `json:"state"`
	Pincode        string        `json:"pincode"`
	Items          []InvoiceItem `json:"items"`
	Subtotal       float64       `json:"subtotal"`
	Total          float64       `json:"total"`
}

// Repository defines the data access interface for invoice data.
type Repository interface {
	GetOrderWithItems(ctx context.Context, orderID int) (Invoice, error)
	GetNextInvoiceNumber(ctx context.Context) (string, error)
	SaveInvoiceNumber(ctx context.Context, orderID int, invoiceNumber string) error
}

// Service handles invoice HTTP endpoints and PDF generation.
type Service struct {
	repo Repository
}

// NewService creates a new invoices service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// RegisterRoutes registers invoice endpoints on the given mux.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /invoices/{id}", auth.WithAuth(http.HandlerFunc(s.getCustomerInvoice)).ServeHTTP)
	mux.HandleFunc("GET /admin/invoices/{id}", auth.WithAuth(auth.WithAdmin(http.HandlerFunc(s.getAdminInvoice))).ServeHTTP)
}

func (s *Service) getCustomerInvoice(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		return
	}

	invoice, err := s.repo.GetOrderWithItems(r.Context(), id)
	if err != nil {
		server.WriteError(w, http.StatusNotFound, "order not found")
		return
	}

	if invoice.Number == "" {
		invoiceNumber, err := s.repo.GetNextInvoiceNumber(r.Context())
		if err != nil {
			server.WriteError(w, http.StatusInternalServerError, "failed to generate invoice number")
			return
		}
		if err := s.repo.SaveInvoiceNumber(r.Context(), id, invoiceNumber); err != nil {
			server.WriteError(w, http.StatusInternalServerError, "failed to save invoice number")
			return
		}
		invoice.Number = invoiceNumber
	}
	invoice.Date = time.Now()

	pdf := generatePDF(invoice)

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=invoice-%d.pdf", id))
	w.Write(pdf)
}

func (s *Service) getAdminInvoice(w http.ResponseWriter, r *http.Request) {
	s.getCustomerInvoice(w, r)
}

func parseID(r *http.Request) (int, error) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}

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
		return Invoice{}, fmt.Errorf("order not found")
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

func (r *repo) GetNextInvoiceNumber(ctx context.Context) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	rows, err := tx.Query(ctx, `SELECT current_number FROM invoice_counters WHERE id = 1`)
	if err != nil {
		return "", err
	}

	var next int64
	if len(rows) == 0 {
		next = 1
		_, err = tx.Execute(ctx, `INSERT INTO invoice_counters (id, current_number) VALUES (1, 1)`)
		if err != nil {
			return "", err
		}
	} else {
		current, _ := rows[0].Int("current_number")
		next = current + 1
		_, err = tx.Execute(ctx, `UPDATE invoice_counters SET current_number = ? WHERE id = 1`, next)
		if err != nil {
			return "", err
		}
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	return formatInvoiceNumber(int(next)), nil
}

func formatInvoiceNumber(n int) string {
	return fmt.Sprintf("INV-%04d", n)
}

func generatePDF(inv Invoice) []byte {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 16)
	pdf.Cell(0, 10, "KR Crackers - Invoice")
	pdf.Ln(12)

	pdf.SetFont("Helvetica", "", 11)
	pdf.Cell(0, 7, fmt.Sprintf("Invoice #: %s", inv.Number))
	pdf.Ln(7)
	pdf.Cell(0, 7, fmt.Sprintf("Date: %s", inv.Date.Format("2006-01-02")))
	pdf.Ln(10)

	pdf.Cell(0, 7, fmt.Sprintf("Customer: %s", inv.CustomerName))
	pdf.Ln(7)
	pdf.Cell(0, 7, fmt.Sprintf("Email: %s", inv.CustomerEmail))
	pdf.Ln(7)
	pdf.Cell(0, 7, fmt.Sprintf("Phone: %s", inv.CustomerPhone))
	pdf.Ln(7)
	pdf.Cell(0, 7, fmt.Sprintf("Address: %s, %s, %s - %s", inv.Street, inv.City, inv.State, inv.Pincode))
	pdf.Ln(12)

	pdf.SetFont("Helvetica", "B", 11)
	pdf.Cell(80, 7, "Product")
	pdf.Cell(20, 7, "Qty")
	pdf.Cell(30, 7, "Unit Price")
	pdf.Cell(30, 7, "Total")
	pdf.Ln(7)
	pdf.SetFont("Helvetica", "", 10)

	for _, item := range inv.Items {
		pdf.Cell(80, 7, item.ProductName)
		pdf.Cell(20, 7, fmt.Sprintf("%d", item.Quantity))
		pdf.Cell(30, 7, fmt.Sprintf("%.2f", item.UnitPrice))
		pdf.Cell(30, 7, fmt.Sprintf("%.2f", item.LineTotal))
		pdf.Ln(7)
	}

	pdf.Ln(5)
	pdf.SetFont("Helvetica", "B", 12)
	pdf.Cell(130, 7, "Total:")
	pdf.Cell(30, 7, fmt.Sprintf("%.2f", inv.Total))

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil
	}
	return buf.Bytes()
}
