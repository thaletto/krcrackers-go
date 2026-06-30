// Package orders provides order lifecycle management including checkout,
// customer order viewing, admin status management, and dashboard statistics.
// Order changes are published to the event bus for notification delivery.
//
// This file contains the business Service. The HTTP layer lives in
// apis/orders.
package orders

import (
	"context"
	"errors"
	"fmt"
	"io"

	apperrors "github.com/thaletto/krcrackers-go/src/errors"
	"github.com/thaletto/krcrackers-go/src/eventbus"
	"github.com/thaletto/krcrackers-go/src/eventbus/events"
	"github.com/thaletto/krcrackers-go/src/services/uploads"
)

// ErrOnlyPendingCancellable is returned by CancelMyOrder when the order
// is no longer in a cancellable state.
var ErrOnlyPendingCancellable = errors.New("only pending orders can be cancelled")

// ErrInvalidStatusTransition is returned by UpdateStatus when the new
// status is not recognised or the transition is not allowed.
var ErrInvalidStatusTransition = errors.New("invalid order status transition")

// Service owns the order business logic: CRUD, checkout, customer
// cancellations, admin status transitions, and dashboard stats. The HTTP
// layer (apis/orders) decodes requests and maps domain errors to status
// codes.
type Service struct {
	repo         Repository
	userProvider UserProvider
	addrProvider AddressProvider
	uploads      UploadsService
	bus          eventbus.Bus
}

// NewService creates a new orders service.
func NewService(
	repo Repository,
	userProvider UserProvider,
	addrProvider AddressProvider,
	uploadsSvc UploadsService,
	bus eventbus.Bus,
) *Service {
	return &Service{
		repo:         repo,
		userProvider: userProvider,
		addrProvider: addrProvider,
		uploads:      uploadsSvc,
		bus:          bus,
	}
}

// Create persists a new order.
func (s *Service) Create(ctx context.Context, input OrderInput) (Order, error) {
	if err := ValidateOrderInput(input); err != nil {
		return Order{}, err
	}
	return s.repo.Create(ctx, input)
}

// List returns a paginated list of all orders.
func (s *Service) List(ctx context.Context, limit, offset int) (ListOrdersResponse, error) {
	return s.repo.List(ctx, limit, offset)
}

// Get returns a single order by ID. Returns apperrors.ErrNotFound if the
// order does not exist.
func (s *Service) Get(ctx context.Context, id int) (Order, error) {
	order, err := s.repo.Get(ctx, id)
	if err != nil {
		return Order{}, err
	}
	if order.ID == 0 {
		return Order{}, apperrors.ErrNotFound
	}
	return order, nil
}

// Update persists changes to an existing order. Returns
// apperrors.ErrNotFound if the order does not exist.
func (s *Service) Update(ctx context.Context, id int, input OrderInput) (Order, error) {
	if err := ValidateOrderInput(input); err != nil {
		return Order{}, err
	}
	order, err := s.repo.Update(ctx, id, input)
	if err != nil {
		return Order{}, err
	}
	if order.ID == 0 {
		return Order{}, apperrors.ErrNotFound
	}
	return order, nil
}

// Delete removes an order by ID. Returns apperrors.ErrNotFound if the
// order does not exist.
func (s *Service) Delete(ctx context.Context, id int) error {
	return s.repo.Delete(ctx, id)
}

// Checkout is the authenticated checkout flow: it resolves the user and
// address, optionally uploads a payment screenshot, computes line totals,
// and persists the order. Returns the order and the uploaded screenshot
// URL (empty if no file was attached).
//
// The caller (HTTP layer) is responsible for parsing the multipart form
// and passing the screenshot reader; this keeps the service free of
// net/http.
func (s *Service) Checkout(
	ctx context.Context,
	userID, addressID int,
	items []CheckoutItem,
	screenshot io.Reader,
	screenshotFilename, screenshotContentType, paymentRef string,
) (Order, string, error) {
	if len(items) == 0 {
		return Order{}, "", fmt.Errorf("at least one item is required")
	}

	var screenshotURL string
	if screenshot != nil {
		if s.uploads == nil {
			return Order{}, "", fmt.Errorf("file uploads not configured")
		}
		key := uploads.GenerateKey("screenshots", screenshotFilename)
		url, err := s.uploads.Put(ctx, key, screenshot, screenshotContentType)
		if err != nil {
			return Order{}, "", fmt.Errorf("upload screenshot: %w", err)
		}
		screenshotURL = url
	}

	userInfo, err := s.userProvider.GetUser(ctx, userID)
	if err != nil {
		return Order{}, "", fmt.Errorf("get user: %w", err)
	}
	if userInfo.ID == 0 {
		return Order{}, "", fmt.Errorf("user not found")
	}

	addr, err := s.addrProvider.GetAddress(ctx, addressID)
	if err != nil {
		return Order{}, "", fmt.Errorf("get address: %w", err)
	}
	if addr.ID == 0 {
		return Order{}, "", fmt.Errorf("address not found")
	}

	orderItems := make([]OrderItemFields, len(items))
	var total float64
	for i, item := range items {
		lineTotal := item.Price * float64(item.Quantity)
		total += lineTotal
		orderItems[i] = OrderItemFields{
			ProductID:   item.ProductID,
			ProductName: item.ProductName,
			Price:       item.Price,
			Quantity:    item.Quantity,
			Total:       lineTotal,
		}
	}

	input := OrderInput{
		OrderFields: OrderFields{
			Status:               StatusPending,
			UserID:               &userID,
			UserName:             userInfo.Name,
			Email:                userInfo.Email,
			Phone:                userInfo.Phone,
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

	order, err := s.repo.Checkout(ctx, input)
	if err != nil {
		return Order{}, "", fmt.Errorf("create order: %w", err)
	}

	s.publishOrderEvent(ctx, events.OrderPlaced, order.ID, userInfo.Phone)
	return order, screenshotURL, nil
}

// ListForUser returns a paginated list of orders for the given customer.
func (s *Service) ListForUser(ctx context.Context, userID, limit, offset int) (ListOrdersResponse, error) {
	return s.repo.ListForUser(ctx, userID, limit, offset)
}

// GetForUser returns a single order scoped to the given customer. Returns
// apperrors.ErrNotFound if the order does not belong to the user.
func (s *Service) GetForUser(ctx context.Context, orderID, userID int) (Order, error) {
	order, err := s.repo.GetForUser(ctx, orderID, userID)
	if err != nil {
		return Order{}, err
	}
	if order.ID == 0 {
		return Order{}, apperrors.ErrNotFound
	}
	return order, nil
}

// CancelMyOrder cancels a pending order owned by the user. Returns
// ErrOnlyPendingCancellable if the order is not pending, or
// apperrors.ErrNotFound if the order does not exist or does not belong
// to the user.
func (s *Service) CancelMyOrder(ctx context.Context, userID, orderID int) (Order, error) {
	order, err := s.GetForUser(ctx, orderID, userID)
	if err != nil {
		return Order{}, err
	}
	if order.Status != StatusPending {
		return Order{}, ErrOnlyPendingCancellable
	}

	updated, err := s.repo.UpdateStatus(ctx, orderID, StatusCancelled)
	if err != nil {
		return Order{}, err
	}

	s.publishOrderEvent(ctx, events.OrderCancelled, updated.ID, updated.Phone)
	return updated, nil
}

// UpdateStatus transitions an order to a new status. Publishes the
// matching event for confirmed / shipped / delivered / cancelled.
// Returns ErrInvalidStatusTransition if the status string is not
// recognised; other errors come from the repository.
func (s *Service) UpdateStatus(ctx context.Context, orderID int, newStatus OrderStatus) (Order, error) {
	order, err := s.repo.UpdateStatus(ctx, orderID, newStatus)
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

// ListAllFilter returns a paginated list of orders, optionally filtered
// by status. An empty status means "all".
func (s *Service) ListAllFilter(ctx context.Context, status string, limit, offset int) (ListOrdersResponse, error) {
	return s.repo.ListAllFilter(ctx, status, limit, offset)
}

// GetDashboardStats returns aggregated dashboard statistics.
func (s *Service) GetDashboardStats(ctx context.Context) (DashboardStats, error) {
	return s.repo.GetDashboardStats(ctx)
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
		return fmt.Errorf("at least one item is required")
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
