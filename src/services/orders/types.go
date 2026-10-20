package orders

import (
	"context"
	"errors"
	"io"
)

// Sentinel errors the HTTP layer maps to statuses. They exist so a handler
// never has to classify a failure by comparing message text.
var (
	// ErrNoItems is returned when an order is submitted with no line items.
	ErrNoItems = errors.New("at least one item is required")

	// ErrAddressNotFound is returned when the delivery address does not
	// exist or does not belong to the shopper checking out.
	ErrAddressNotFound = errors.New("address not found")

	// ErrShopperNotFound is returned when the shopper on the session no
	// longer has an account.
	ErrShopperNotFound = errors.New("user not found")

	// ErrUploadsUnavailable is returned when a payment screenshot was
	// attached but file storage is not configured.
	ErrUploadsUnavailable = errors.New("file uploads not configured")

	// ErrIntakeUnavailable is returned when an order is placed but the
	// modules intake depends on were not wired. It signals a wiring fault,
	// not a client error.
	ErrIntakeUnavailable = errors.New("order intake is not configured")

	// ErrUnknownProduct is returned when a basket names a product the
	// catalogue does not have.
	ErrUnknownProduct = errors.New("product not found")

	// ErrInvalidQuantity is returned when a basket line asks for a quantity
	// that is not positive.
	ErrInvalidQuantity = errors.New("quantity must be at least 1")

	// ErrTooManyItems is returned when a basket has more lines than an order
	// may hold. Each line is one INSERT, and on D1 each of those is a separate
	// HTTP round trip, so an unbounded count is a request to make the server do
	// tens of thousands of calls on an unauthenticated route.
	ErrTooManyItems = errors.New("too many items in one order")

	// ErrImmutableFields is returned when a payload tries to set a field the
	// order module owns. Status moves through UpdateStatus so the lifecycle and
	// its events apply; user_id comes from the session, never a body.
	ErrImmutableFields = errors.New("status and userId cannot be set on this request")
)

// MaxItemsPerOrder bounds the line items one order may carry.
const MaxItemsPerOrder = 50

// Intake bundles what order intake needs from the modules around it. Taking
// them as one value keeps the constructor honest and replaces a positional
// list where forgetting an argument compiles fine and panics at the call.
type Intake struct {
	// Shoppers resolves the account placing the order.
	Shoppers UserProvider

	// Addresses resolves the delivery address.
	Addresses AddressProvider

	// Catalogue prices the basket. Without it an order cannot be placed,
	// because the price is the one thing intake must not take from the
	// client.
	Catalogue Catalogue

	// Uploads stores payment screenshots. Optional: an order without a
	// screenshot is fine, so its absence is not a wiring fault.
	Uploads UploadsService
}

// PricedProduct is what the catalogue holds for one product.
type PricedProduct struct {
	Name  string
	Price float64
}

// Catalogue is the seam onto the Product module. The Order module asks it what
// a product costs rather than reading the products table, and it ignores the
// price in the request: what a shopper pays is the Product module's figure.
//
// Prices(ctx, ids) returns only the ids the catalogue knows; an id missing
// from the map is a product that does not exist.
type Catalogue interface {
	Prices(ctx context.Context, productIDs []int) (map[int]PricedProduct, error)
}

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
	Price       float64 `json:"price"`
	Total       float64 `json:"total"`
	ProductID   int     `json:"productId"`
	Quantity    int     `json:"quantity"`
	ProductName string  `json:"productName"`
}

// OrderItem is an order line item as it comes back from the repository.
type OrderItem struct {
	ID int `json:"id"`
	OrderItemFields
}

// OrderFields contains the data fields for an order header.
type OrderFields struct {
	Total                float64     `json:"total"`
	Status               OrderStatus `json:"status"`
	UserName             string      `json:"userName"`
	Email                string      `json:"email"`
	Phone                string      `json:"phone"`
	Street               string      `json:"street"`
	TownOrCity           string      `json:"townOrCity"`
	State                string      `json:"state"`
	Pincode              string      `json:"pincode"`
	DeliveryRegion       string      `json:"deliveryRegion"`
	DeliveryLocation     string      `json:"deliveryLocation"`
	PaymentScreenshotURL string      `json:"paymentScreenshotUrl,omitempty"`
	PaymentReference     string      `json:"paymentReference,omitempty"`
	UserID               *int        `json:"userId,omitempty"`
	Notes                *string     `json:"notes,omitempty"`
}

// Order represents a complete order with its line items.
type Order struct {
	ID int `json:"id"`
	OrderFields
	Items     []OrderItem `json:"items"`
	CreatedAt string      `json:"createdAt,omitempty"`
}

// OrderInput is the request payload for creating or updating an order.
type OrderInput struct {
	OrderFields
	Items []OrderItemFields `json:"items"`
}

// ListFilter narrows and paginates an order list. A zero Limit means
// "omitted": every matching order is returned with no LIMIT clause.
type ListFilter struct {
	Status string
	Limit  int
	Offset int
}

// ListOrdersResponse is the paginated response for order list endpoints.
type ListOrdersResponse struct {
	Items  []Order `json:"items"`
	Total  int     `json:"total"`
	Limit  *int    `json:"limit,omitempty"`
	Offset *int    `json:"offset,omitempty"`
}

// InvoiceView is the flat order snapshot an invoice renders. It exists so
// the Invoice module never reads the orders table: it asks the Order module
// for one of these, already resolved against the caller's Actor.
type InvoiceView struct {
	OrderID    int
	Number     string
	UserName   string
	Email      string
	Phone      string
	Street     string
	TownOrCity string
	State      string
	Pincode    string
	Total      float64
	Items      []InvoiceLine
}

// InvoiceLine is one line of an InvoiceView.
type InvoiceLine struct {
	ProductName string
	Quantity    int
	Price       float64
	Total       float64
}

// DashboardStats contains the order figures for the admin dashboard. The
// new-customer count is not here: it belongs to the Account module, and the
// route composes the two rather than the Order module reaching into users.
type DashboardStats struct {
	TotalOrders   int     `json:"totalOrders"`
	PendingOrders int     `json:"pendingOrders"`
	RevenueMonth  float64 `json:"revenueMonth"`
}

// CheckoutItem is a single line item in a checkout request, as submitted
// by the client.
type CheckoutItem struct {
	Price       float64 `json:"price"`
	ProductID   int     `json:"productId"`
	Quantity    int     `json:"quantity"`
	ProductName string  `json:"productName"`
}

// UserProvider is an interface for fetching user data, used during checkout.
type UserProvider interface {
	GetUser(ctx context.Context, id int) (User, error)
}

// AddressProvider is an interface for fetching a delivery address during
// checkout. The lookup is scoped to the shopper: an address belonging to
// somebody else is ErrAddressNotFound, the same as one that does not exist, so
// the ownership decision is the provider's SQL rather than this module's
// arithmetic.
type AddressProvider interface {
	AddressFor(ctx context.Context, shopperID, addressID int) (Address, error)
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
