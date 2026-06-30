package orders

import (
	"context"
	"io"
)

// OrderStatus represents the lifecycle state of an order.
type OrderStatus string

// Valid order status values and their allowed transitions:
//
//	pending -> confirmed, cancelled
//	confirmed -> shipped, cancelled
//	shipped -> delivered, cancelled
//	delivered (terminal)
//	cancelled (terminal)
const (
	StatusPending   OrderStatus = "pending"
	StatusConfirmed OrderStatus = "confirmed"
	StatusShipped   OrderStatus = "shipped"
	StatusDelivered OrderStatus = "delivered"
	StatusCancelled OrderStatus = "cancelled"
)

// OrderItemFields contains the data fields for an order line item.
type OrderItemFields struct {
	ProductID   int     `json:"productId"`
	ProductName string  `json:"productName"`
	Price       float64 `json:"price"`
	Quantity    int     `json:"quantity"`
	Total       float64 `json:"total"`
}

// OrderItem is an order line item as it comes back from the repository.
type OrderItem struct {
	ID int `json:"id"`
	OrderItemFields
}

// OrderFields contains the data fields for an order header.
type OrderFields struct {
	Status               OrderStatus `json:"status"`
	UserID               *int        `json:"userId,omitempty"`
	UserName             string      `json:"userName"`
	Email                string      `json:"email"`
	Phone                string      `json:"phone"`
	Street               string      `json:"street"`
	TownOrCity           string      `json:"townOrCity"`
	State                string      `json:"state"`
	Pincode              string      `json:"pincode"`
	Notes                *string     `json:"notes,omitempty"`
	DeliveryRegion       string      `json:"deliveryRegion"`
	DeliveryLocation     string      `json:"deliveryLocation"`
	Total                float64     `json:"total"`
	PaymentScreenshotURL string      `json:"paymentScreenshotUrl,omitempty"`
	PaymentReference     string      `json:"paymentReference,omitempty"`
}

// Order represents a complete order with its line items.
type Order struct {
	ID        int         `json:"id"`
	OrderFields
	Items     []OrderItem `json:"items"`
	CreatedAt string      `json:"createdAt,omitempty"`
}

// OrderInput is the request payload for creating or updating an order.
type OrderInput struct {
	OrderFields
	Items []OrderItemFields `json:"items"`
}

// ListOrdersResponse is the paginated response for order list endpoints.
type ListOrdersResponse struct {
	Items  []Order `json:"items"`
	Total  int     `json:"total"`
	Limit  *int    `json:"limit,omitempty"`
	Offset *int    `json:"offset,omitempty"`
}

// DashboardStats contains aggregated order statistics for the admin dashboard.
type DashboardStats struct {
	TotalOrders   int     `json:"totalOrders"`
	PendingOrders int     `json:"pendingOrders"`
	RevenueMonth  float64 `json:"revenueMonth"`
	NewCustomers  int     `json:"newCustomers"`
}

// CheckoutItem is a single line item in a checkout request, as submitted
// by the client.
type CheckoutItem struct {
	ProductID   int     `json:"productId"`
	ProductName string  `json:"productName"`
	Price       float64 `json:"price"`
	Quantity    int     `json:"quantity"`
}

// UserProvider is an interface for fetching user data, used during checkout.
type UserProvider interface {
	GetUser(ctx context.Context, id int) (User, error)
}

// AddressProvider is an interface for fetching customer addresses, used
// during checkout.
type AddressProvider interface {
	GetAddress(ctx context.Context, id int) (Address, error)
}

// UploadsService is an interface for file uploads, used for payment
// screenshots.
type UploadsService interface {
	Put(ctx context.Context, key string, body io.Reader, contentType string) (string, error)
}

// User holds minimal user data needed for order creation.
type User struct {
	ID    int
	Email string
	Name  string
	Phone string
}

// Address holds shipping address data snapshotted at checkout time.
type Address struct {
	ID      int
	UserID  int
	Label   string
	Street  string
	City    string
	State   string
	Pincode string
	Country string
}
