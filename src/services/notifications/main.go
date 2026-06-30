// Package notifications provides order notification delivery via the
// WhatsApp Cloud API. Falls back to a no-op service when credentials
// are not configured.
package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Service defines the interface for sending order notifications.
type Service interface {
	SendOrderPlaced(ctx context.Context, phone string, orderID int)
	SendPaymentConfirmed(ctx context.Context, phone string, orderID int)
	SendOrderShipped(ctx context.Context, phone string, orderID int)
	SendOrderDelivered(ctx context.Context, phone string, orderID int)
	SendOrderCancelled(ctx context.Context, phone string, orderID int)
}

type whatsappClient struct {
	apiToken      string
	phoneNumberID string
	fromNumber    string
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

// NewWhatsAppService returns a WhatsApp client if credentials are provided,
// otherwise returns a no-op service that silently drops all notifications.
func NewWhatsAppService(apiToken, phoneNumberID, fromNumber string) Service {
	if apiToken == "" || phoneNumberID == "" {
		return &noopService{}
	}
	return &whatsappClient{apiToken: apiToken, phoneNumberID: phoneNumberID, fromNumber: fromNumber}
}

func (w *whatsappClient) sendTemplate(ctx context.Context, to, templateName string, params []string) {
	components := []map[string]any{
		{
			"type": "body",
			"parameters": func() []map[string]any {
				ps := make([]map[string]any, len(params))
				for i, p := range params {
					ps[i] = map[string]any{"type": "text", "text": p}
				}
				return ps
			}(),
		},
	}

	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "template",
		"template": map[string]any{
			"name":       templateName,
			"language":   map[string]string{"code": "en"},
			"components": components,
		},
	}

	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/messages", w.phoneNumberID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		log.Printf("whatsapp: failed to create request: %v", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+w.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("whatsapp: failed to send: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		log.Printf("whatsapp: send failed with status %d", resp.StatusCode)
	}
}

func (w *whatsappClient) SendOrderPlaced(ctx context.Context, phone string, orderID int) {
	w.sendTemplate(ctx, phone, "order_placed", []string{fmt.Sprintf("%d", orderID)})
}

func (w *whatsappClient) SendPaymentConfirmed(ctx context.Context, phone string, orderID int) {
	w.sendTemplate(ctx, phone, "payment_confirmed", []string{fmt.Sprintf("%d", orderID)})
}

func (w *whatsappClient) SendOrderShipped(ctx context.Context, phone string, orderID int) {
	w.sendTemplate(ctx, phone, "order_shipped", []string{fmt.Sprintf("%d", orderID)})
}

func (w *whatsappClient) SendOrderDelivered(ctx context.Context, phone string, orderID int) {
	w.sendTemplate(ctx, phone, "order_delivered", []string{fmt.Sprintf("%d", orderID)})
}

func (w *whatsappClient) SendOrderCancelled(ctx context.Context, phone string, orderID int) {
	w.sendTemplate(ctx, phone, "order_cancelled", []string{fmt.Sprintf("%d", orderID)})
}

type noopService struct{}

func (n *noopService) SendOrderPlaced(_ context.Context, _ string, _ int)       {}
func (n *noopService) SendPaymentConfirmed(_ context.Context, _ string, _ int)  {}
func (n *noopService) SendOrderShipped(_ context.Context, _ string, _ int)      {}
func (n *noopService) SendOrderDelivered(_ context.Context, _ string, _ int)    {}
func (n *noopService) SendOrderCancelled(_ context.Context, _ string, _ int)    {}
