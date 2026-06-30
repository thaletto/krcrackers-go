package invoices

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/jung-kurt/gofpdf"
)

// Service owns the business operations for invoice generation: loading
// the order, deriving and persisting the invoice number, and rendering
// the PDF. The HTTP layer (apis/invoices) writes the bytes and headers.
type Service struct {
	repo Repository
}

// NewService creates a new invoices service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// GenerateResult bundles the PDF bytes with a suggested filename.
type GenerateResult struct {
	PDF      []byte
	Filename string
}

// GeneratePDF loads the order, ensures it has an invoice number, and
// renders the PDF. Returns ErrOrderNotFound if the order does not exist.
func (s *Service) GeneratePDF(ctx context.Context, orderID int) (GenerateResult, error) {
	invoice, err := s.repo.GetOrderWithItems(ctx, orderID)
	if err != nil {
		return GenerateResult{}, err
	}

	if invoice.Number == "" {
		invoiceNumber := formatInvoiceNumber(orderID)
		if err := s.repo.SaveInvoiceNumber(ctx, orderID, invoiceNumber); err != nil {
			return GenerateResult{}, fmt.Errorf("save invoice number: %w", err)
		}
		invoice.Number = invoiceNumber
	}
	invoice.Date = time.Now()

	pdf := generatePDF(invoice)
	return GenerateResult{
		PDF:      pdf,
		Filename: fmt.Sprintf("invoice-%d.pdf", orderID),
	}, nil
}

func formatInvoiceNumber(orderID int) string {
	return fmt.Sprintf("INV-%04d", orderID)
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
