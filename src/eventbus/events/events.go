// Package events defines event name constants and payload types used
// across the event bus for cross-service communication.
package events

// Event name constants for product lifecycle.
const (
	ProductCreated = "product.created"
	ProductUpdated = "product.updated"
	ProductDeleted = "product.deleted"

	OrderPlaced    = "order.placed"
	OrderConfirmed = "order.confirmed"
	OrderShipped   = "order.shipped"
	OrderDelivered = "order.delivered"
	OrderCancelled = "order.cancelled"
)

// ProductEvent carries product data for product lifecycle events.
type ProductEvent struct {
	ID           int
	Name         string
	Description  string
	Price        float64
	Brand        string
	Category     string
	Image        string
	ComparePrice float64
	Rating       *float64
	Delivery     *string
}

// OrderEvent carries order data for order lifecycle events.
type OrderEvent struct {
	OrderID int
	Phone   string
	Status  string
}
