package invoices

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jung-kurt/gofpdf"

	"github.com/thaletto/krcrackers-go/src/services/orders"
)

// Service owns the business operations for invoice generation: resolving the
// order through the Order module, deriving and recording the invoice number,
// and rendering the PDF. The HTTP layer (apis/invoices) writes the bytes and
// headers.
type Service struct {
	orders OrderSource
}

// NewService creates a new invoices service over the Order module.
func NewService(orderSource OrderSource) *Service {
	return &Service{orders: orderSource}
}

// GenerateResult bundles the PDF bytes with a suggested filename.
type GenerateResult struct {
	PDF      []byte
	Filename string
}

// GeneratePDF renders the invoice for an order the actor may see. The lookup
// and the invoice-number write both go through the Order module, so an
// actor who cannot see the order cannot obtain its invoice. Returns
// apperrors.ErrNotFound when the order does not exist or is out of scope.
func (s *Service) GeneratePDF(ctx context.Context, actor orders.Actor, orderID int) (GenerateResult, error) {
	view, err := s.orders.InvoiceView(ctx, actor, orderID)
	if err != nil {
		return GenerateResult{}, err
	}

	number := view.Number
	if number == "" {
		number = formatInvoiceNumber(orderID)
		if err := s.orders.AssignInvoiceNumber(ctx, actor, orderID, number); err != nil {
			return GenerateResult{}, fmt.Errorf("save invoice number: %w", err)
		}
	}

	pdf := generatePDF(render(view, number, time.Now()))
	return GenerateResult{
		PDF:      pdf,
		Filename: fmt.Sprintf("invoice-%d.pdf", orderID),
	}, nil
}

// bufPool reuses PDF output buffers across requests.
var bufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

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

	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)
	if err := pdf.Output(buf); err != nil {
		return nil
	}
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out
}
