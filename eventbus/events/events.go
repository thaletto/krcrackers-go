package events

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

type ProductEvent struct {
	ID           int
	Name         string
	Description  string
	Price        float64
	Brand        string
	Category     string
	Image        string
	ComparePrice float64
}

type OrderEvent struct {
	OrderID int
	Phone   string
	Status  string
}
