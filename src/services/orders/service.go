// Package orders provides order lifecycle management including checkout,
// customer order viewing, admin status management, and dashboard statistics.
// Order changes are published to the event bus for notification delivery.
//
// Every read and write of an order is resolved against an Actor, so who is
// asking is part of the call rather than something a caller remembers to
// check. The HTTP layer lives in apis/orders.
package orders

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	apperrors "github.com/thaletto/krcrackers-go/src/errors"
	"github.com/thaletto/krcrackers-go/src/eventbus"
	"github.com/thaletto/krcrackers-go/src/eventbus/events"
	"github.com/thaletto/krcrackers-go/src/services/uploads"
)

// ErrOnlyPendingCancellable is returned by Cancel when the order is no
// longer in a cancellable state.
var ErrOnlyPendingCancellable = errors.New("only pending orders can be cancelled")

// ErrInvalidStatusTransition is returned by UpdateStatus when the new status
// is not recognised or the transition is not allowed by the lifecycle.
var ErrInvalidStatusTransition = errors.New("invalid order status transition")

// Service owns the order business logic: intake, retrieval, cancellations,
// admin status transitions, and dashboard stats. It is the only way to reach
// an order: every method takes the Actor asking for it, and no method
// bypasses the ownership check. The HTTP layer (apis/orders) decodes requests
// and maps domain errors to status codes.
type Service struct {
	repo   Repository
	intake Intake
	bus    eventbus.Bus
}

// NewService creates a new orders service. The Intake value names where the
// surrounding modules come from; bus may be nil when nothing subscribes.
func NewService(repo Repository, intake Intake, bus eventbus.Bus) *Service {
	return &Service{repo: repo, intake: intake, bus: bus}
}

// priceBasket resolves a submitted basket to catalogue prices.
//
// The price and product name in the request are ignored. A client that
// quotes a lower price gets the catalogue's price, which is the only way the
// Order module can be sure what it is selling. A product the catalogue does
// not have, or a line asking for no items, is refused rather than stored.
func (s *Service) priceBasket(ctx context.Context, items []OrderItemFields) ([]OrderItemFields, float64, error) {
	if len(items) == 0 {
		return nil, 0, ErrNoItems
	}
	if len(items) > MaxItemsPerOrder {
		return nil, 0, fmt.Errorf("%w: %d, limit is %d", ErrTooManyItems, len(items), MaxItemsPerOrder)
	}
	if s.intake.Catalogue == nil {
		return nil, 0, ErrIntakeUnavailable
	}

	ids := make([]int, 0, len(items))
	for _, item := range items {
		if item.Quantity < 1 {
			return nil, 0, fmt.Errorf("%w: product %d", ErrInvalidQuantity, item.ProductID)
		}
		ids = append(ids, item.ProductID)
	}

	prices, err := s.intake.Catalogue.Prices(ctx, ids)
	if err != nil {
		return nil, 0, fmt.Errorf("price basket: %w", err)
	}

	lines := make([]OrderItemFields, 0, len(items))
	var total float64
	for _, item := range items {
		product, ok := prices[item.ProductID]
		if !ok {
			return nil, 0, fmt.Errorf("%w: %d", ErrUnknownProduct, item.ProductID)
		}
		line := OrderItemFields{
			ProductID:   item.ProductID,
			ProductName: product.Name,
			Price:       product.Price,
			Quantity:    item.Quantity,
			Total:       product.Price * float64(item.Quantity),
		}
		total += line.Total
		lines = append(lines, line)
	}
	return lines, total, nil
}

// Create persists a new order on behalf of the Actor.
//
// The owner is taken from the Actor rather than the payload, so a client
// cannot file an order under somebody else's account by setting userId in
// the body. A guest's order stays unowned and is not addressable by id. Line
// items are re-priced from the catalogue, so the totals in the payload are
// only a request.
func (s *Service) Create(ctx context.Context, input OrderInput, actor Actor) (Order, error) {
	if err := ValidateOrderInput(input); err != nil {
		return Order{}, err
	}

	items, total, err := s.priceBasket(ctx, input.Items)
	if err != nil {
		return Order{}, err
	}
	input.Items = items
	input.Total = total

	if actor.IsGuest() {
		input.UserID = nil
	} else {
		owner := actor.UserID
		input.UserID = &owner
	}
	return s.repo.Insert(ctx, input)
}

// List returns a page of the orders the Actor may see: an administrator's
// whole book, a shopper's own history. filter.Status is applied to both.
func (s *Service) List(ctx context.Context, actor Actor, filter ListFilter) (ListOrdersResponse, error) {
	return s.repo.List(ctx, filter, actor.scope())
}

// Get returns one order the Actor may see. Returns
// apperrors.ErrNotFound when the order does not exist or belongs to somebody
// else, so a caller cannot probe for the existence of another account's
// orders.
func (s *Service) Get(ctx context.Context, actor Actor, id int) (Order, error) {
	order, err := s.repo.Get(ctx, id, actor.scope())
	if err != nil {
		return Order{}, err
	}
	if order.ID == 0 {
		return Order{}, apperrors.ErrNotFound
	}
	return order, nil
}

// Update persists changes to an order the Actor may edit. As with intake, the
// line items and total are re-priced from the catalogue, so editing an order
// is not a way to change what it cost.
//
// A payload may not carry `status` or `userId`. The first moves an order through
// UpdateStatus, where the lifecycle and its event apply; the second is the
// owner, which comes from the session. Both used to be accepted in the body and
// then silently dropped by the write, so a caller who set them got a 200 and no
// change. Refusing is the honest answer.
//
// Returns apperrors.ErrNotFound if the order does not exist or is out of scope.
func (s *Service) Update(ctx context.Context, actor Actor, id int, input OrderInput) (Order, error) {
	if input.Status != "" {
		return Order{}, ErrImmutableFields
	}
	if input.UserID != nil {
		return Order{}, ErrImmutableFields
	}
	if err := ValidateOrderInput(input); err != nil {
		return Order{}, err
	}
	items, total, err := s.priceBasket(ctx, input.Items)
	if err != nil {
		return Order{}, err
	}
	input.Items = items
	input.Total = total

	order, err := s.repo.Update(ctx, id, input, actor.scope())
	if err != nil {
		return Order{}, err
	}
	if order.ID == 0 {
		return Order{}, apperrors.ErrNotFound
	}
	return order, nil
}

// Delete removes an order the Actor may delete. Returns
// apperrors.ErrNotFound if the order does not exist or is out of scope.
func (s *Service) Delete(ctx context.Context, actor Actor, id int) error {
	return s.repo.Delete(ctx, id, actor.scope())
}

// Checkout is the authenticated checkout flow: it prices the basket from the
// catalogue, resolves the shopper and their address, optionally uploads a
// payment screenshot, and persists the order. Returns the order and the
// uploaded screenshot URL (empty if no file was attached).
//
// The caller (HTTP layer) is responsible for parsing the multipart form
// and passing the screenshot reader; this keeps the service free of
// net/http.
func (s *Service) Checkout(
	ctx context.Context,
	actor Actor,
	addressID int,
	items []CheckoutItem,
	screenshot io.Reader,
	screenshotFilename, screenshotContentType, paymentRef string,
) (Order, string, error) {
	if len(items) == 0 {
		return Order{}, "", ErrNoItems
	}
	if actor.IsGuest() {
		return Order{}, "", ErrNotPermitted
	}
	if s.intake.Shoppers == nil || s.intake.Addresses == nil || s.intake.Catalogue == nil {
		return Order{}, "", ErrIntakeUnavailable
	}

	// Price before uploading anything: a basket that cannot be priced should
	// not leave a payment screenshot sitting in storage.
	submitted := make([]OrderItemFields, len(items))
	for i, item := range items {
		submitted[i] = OrderItemFields{ProductID: item.ProductID, Quantity: item.Quantity}
	}
	orderItems, total, err := s.priceBasket(ctx, submitted)
	if err != nil {
		return Order{}, "", err
	}

	var screenshotURL string
	if screenshot != nil {
		if s.intake.Uploads == nil {
			return Order{}, "", ErrUploadsUnavailable
		}
		key := uploads.GenerateKey("screenshots", screenshotFilename)
		url, err := s.intake.Uploads.Put(ctx, key, screenshot, screenshotContentType)
		if err != nil {
			return Order{}, "", fmt.Errorf("upload screenshot: %w", err)
		}
		screenshotURL = url
	}

	shopper, err := s.intake.Shoppers.GetUser(ctx, actor.UserID)
	if err != nil {
		return Order{}, "", fmt.Errorf("get user: %w", err)
	}
	if shopper.ID == 0 {
		return Order{}, "", ErrShopperNotFound
	}

	addr, err := s.intake.Addresses.AddressFor(ctx, actor.UserID, addressID)
	if err != nil {
		if errors.Is(err, ErrAddressNotFound) {
			return Order{}, "", ErrAddressNotFound
		}
		return Order{}, "", fmt.Errorf("get address: %w", err)
	}
	// The provider scopes the lookup to the shopper, so this is a second
	// check on an invariant rather than the only one.
	if addr.ID == 0 || addr.UserID != actor.UserID {
		return Order{}, "", ErrAddressNotFound
	}

	owner := actor.UserID
	input := OrderInput{
		OrderFields: OrderFields{
			Status:               StatusPending,
			UserID:               &owner,
			UserName:             shopper.Name,
			Email:                shopper.Email,
			Phone:                shopper.Phone,
			Street:               addr.Street,
			TownOrCity:           addr.City,
			State:                addr.State,
			Pincode:              addr.Pincode,
			DeliveryRegion:       addr.State,
			DeliveryLocation:     addr.City,
			Total:                total,
			PaymentScreenshotURL: screenshotURL,
			PaymentReference:     paymentRef,
		},
		Items: orderItems,
	}

	order, err := s.repo.Insert(ctx, input)
	if err != nil {
		return Order{}, "", fmt.Errorf("create order: %w", err)
	}

	s.publishOrderEvent(ctx, events.OrderPlaced, order.ID, shopper.Phone)
	return order, screenshotURL, nil
}

// Cancel cancels a pending order the Actor owns. Returns
// ErrOnlyPendingCancellable if the order is not pending, or
// apperrors.ErrNotFound if the order does not exist or is out of scope.
func (s *Service) Cancel(ctx context.Context, actor Actor, orderID int) (Order, error) {
	order, err := s.Get(ctx, actor, orderID)
	if err != nil {
		return Order{}, err
	}
	if order.Status != StatusPending {
		return Order{}, ErrOnlyPendingCancellable
	}

	updated, err := s.repo.UpdateStatus(ctx, orderID, StatusCancelled, actor.scope())
	if err != nil {
		return Order{}, err
	}

	s.publishOrderEvent(ctx, events.OrderCancelled, updated.ID, updated.Phone)
	return updated, nil
}

// UpdateStatus transitions an order to a new status. Administrator-only: any
// other Actor gets ErrNotPermitted, so a route registered without the admin
// middleware still cannot move an order. Publishes the matching event for
// confirmed / shipped / delivered / cancelled.
func (s *Service) UpdateStatus(ctx context.Context, actor Actor, orderID int, newStatus OrderStatus) (Order, error) {
	if !actor.CanAdminister() {
		return Order{}, ErrNotPermitted
	}
	order, err := s.repo.UpdateStatus(ctx, orderID, newStatus, AnyOrder())
	if err != nil {
		return Order{}, err
	}

	var eventName string
	switch newStatus {
	case StatusConfirmed:
		eventName = events.OrderConfirmed
	case StatusShipped:
		eventName = events.OrderShipped
	case StatusDelivered:
		eventName = events.OrderDelivered
	case StatusCancelled:
		eventName = events.OrderCancelled
	}
	if eventName != "" {
		s.publishOrderEvent(ctx, eventName, order.ID, order.Phone)
	}
	return order, nil
}

// DashboardStats returns the order figures for the admin dashboard since the
// first instant of the current month. Administrator-only.
func (s *Service) DashboardStats(ctx context.Context, actor Actor) (DashboardStats, error) {
	if !actor.CanAdminister() {
		return DashboardStats{}, ErrNotPermitted
	}
	return s.repo.DashboardStats(ctx, startOfMonth(time.Now().UTC()))
}

// startOfMonth is the first instant of the month containing t. The month
// boundary is computed here rather than by the database, so the figure is a
// function of a clock the caller controls and not of a dialect's date
// modifiers.
func startOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// InvoiceView returns the order snapshot an invoice renders, for an order
// the Actor may see. The Invoice module uses this instead of reading the
// orders table, so ownership is decided in one place.
func (s *Service) InvoiceView(ctx context.Context, actor Actor, orderID int) (InvoiceView, error) {
	return s.repo.InvoiceView(ctx, orderID, actor.scope())
}

// AssignInvoiceNumber records the invoice number minted for an order the
// Actor may see.
func (s *Service) AssignInvoiceNumber(ctx context.Context, actor Actor, orderID int, number string) error {
	return s.repo.AssignInvoiceNumber(ctx, orderID, number, actor.scope())
}

// ValidateOrderInput checks the minimal required fields on a new or
// updated order.
func ValidateOrderInput(o OrderInput) error {
	if o.UserName == "" {
		return fmt.Errorf("userName is required")
	}
	if o.Email == "" {
		return fmt.Errorf("email is required")
	}
	if o.Phone == "" {
		return fmt.Errorf("phone is required")
	}
	if o.Street == "" {
		return fmt.Errorf("street is required")
	}
	if o.TownOrCity == "" {
		return fmt.Errorf("townOrCity is required")
	}
	if o.State == "" {
		return fmt.Errorf("state is required")
	}
	if o.Pincode == "" {
		return fmt.Errorf("pincode is required")
	}
	if o.DeliveryRegion == "" {
		return fmt.Errorf("deliveryRegion is required")
	}
	if o.DeliveryLocation == "" {
		return fmt.Errorf("deliveryLocation is required")
	}
	if len(o.Items) == 0 {
		return ErrNoItems
	}
	return nil
}

func (s *Service) publishOrderEvent(ctx context.Context, eventName string, orderID int, phone string) {
	if s.bus == nil {
		return
	}
	_ = s.bus.Publish(ctx, eventbus.Event{
		Name: eventName,
		Payload: events.OrderEvent{
			OrderID: orderID,
			Phone:   phone,
			Status:  eventName,
		},
	})
}
