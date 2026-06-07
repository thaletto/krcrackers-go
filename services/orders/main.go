package orders

import (
	"context"
	"log"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/thaletto/krcrackers-go/database"
)

type Service struct {
	db database.DB
}

type OrderItemFields struct {
	ProductID   int     `json:"productId" required:"true" minimum:"1" example:"1"`
	ProductName string  `json:"productName" required:"true" minLength:"1" example:"Sneaker"`
	Price       float64 `json:"price" required:"true" minimum:"0" example:"99"`
	Quantity    int     `json:"quantity" required:"true" minimum:"1" example:"2"`
	Total       float64 `json:"total" required:"true" minimum:"0" example:"198"`
}

type OrderItem struct {
	ID int `json:"id" example:"1"`
	OrderItemFields
}

type OrderFields struct {
	UserName         string  `json:"userName" required:"true" minLength:"1" example:"John Doe"`
	Email            string  `json:"email" required:"true" minLength:"1" example:"john@example.com"`
	Phone            string  `json:"phone" required:"true" minLength:"1" example:"9876543210"`
	Street           string  `json:"street" required:"true" minLength:"1" example:"123 Main St"`
	TownOrCity       string  `json:"townOrCity" required:"true" minLength:"1" example:"Mumbai"`
	State            string  `json:"state" required:"true" minLength:"1" example:"Maharashtra"`
	Pincode          string  `json:"pincode" required:"true" minLength:"1" example:"400001"`
	Notes            *string `json:"notes,omitempty" nullable:"true" example:"Ring the doorbell"`
	DeliveryRegion   string  `json:"deliveryRegion" required:"true" minLength:"1" example:"West"`
	DeliveryLocation string  `json:"deliveryLocation" required:"true" minLength:"1" example:"Front Door"`
	Total            float64 `json:"total" required:"true" minimum:"0" example:"297"`
}

type Order struct {
	ID int `json:"id" example:"1"`
	OrderFields
	Items []OrderItem `json:"items"`
}

type OrderInput struct {
	OrderFields
	Items []OrderItemFields `json:"items" required:"true" minItems:"1"`
}

type CreateOrderInput struct {
	Body OrderInput
}
type CreateOrderOutput struct {
	Body Order
}

type ListOrdersInput struct {
	Limit  int `query:"limit" required:"false" maximum:"100" example:"20"`
	Offset int `query:"offset" required:"false" minimum:"0" example:"0"`
}

type ListOrdersResponse struct {
	Items  []Order `json:"items"`
	Total  int     `json:"total" example:"100"`
	Limit  *int    `json:"limit" nullable:"true" example:"20"`
	Offset *int    `json:"offset" nullable:"true" example:"0"`
}

type ListOrdersOutput struct {
	Body ListOrdersResponse
}

type GetOrderInput struct {
	ID int `path:"id" required:"true" minimum:"1" example:"1"`
}
type GetOrderOutput struct {
	Body Order
}

type UpdateOrderInput struct {
	ID   int `path:"id" required:"true" minimum:"1" example:"1"`
	Body OrderInput
}
type UpdateOrderOutput struct {
	Body Order
}

type DeleteOrderInput struct {
	ID int `path:"id" required:"true" minimum:"1" example:"1"`
}

func NewService(db database.DB) *Service {
	return &Service{db: db}
}

func (s *Service) RegisterRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-order",
		Method:      http.MethodPost,
		Path:        "/orders",
		Summary:     "Create an order",
		Description: "Places a new order with one or more items.",
		Tags:        []string{"orders"},
	}, s.create)

	huma.Register(api, huma.Operation{
		OperationID: "list-orders",
		Method:      http.MethodGet,
		Path:        "/orders",
		Summary:     "List orders",
		Description: "Returns a page of orders ordered by id. Use `limit` and `offset` query parameters to paginate; the response includes the total row count.",
		Tags:        []string{"orders"},
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID: "get-order",
		Method:      http.MethodGet,
		Path:        "/orders/{id}",
		Summary:     "Get an order",
		Description: "Returns a single order by id, including all its items.",
		Tags:        []string{"orders"},
	}, s.get)

	huma.Register(api, huma.Operation{
		OperationID: "update-order",
		Method:      http.MethodPut,
		Path:        "/orders/{id}",
		Summary:     "Update an order",
		Description: "Replaces an existing order and its items. Returns 404 if the order does not exist.",
		Tags:        []string{"orders"},
	}, s.update)

	huma.Register(api, huma.Operation{
		OperationID: "delete-order",
		Method:      http.MethodDelete,
		Path:        "/orders/{id}",
		Summary:     "Delete an order",
		Description: "Removes an order and its items. Returns 404 if the order does not exist.",
		Tags:        []string{"orders"},
	}, s.delete)
}

func (s *Service) create(ctx context.Context, in *CreateOrderInput) (*CreateOrderOutput, error) {
	b := in.Body
	res, err := s.db.Execute(ctx, `
		INSERT INTO orders (user_name, email, phone, street, town_or_city, state, pincode, notes, delivery_region, delivery_location, total)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, b.UserName, b.Email, b.Phone, b.Street, b.TownOrCity, b.State, b.Pincode, b.Notes, b.DeliveryRegion, b.DeliveryLocation, b.Total)
	if err != nil {
		log.Printf("insert order: %v", err)
		return nil, huma.Error500InternalServerError("failed to create order")
	}
	orderID := int(res.LastInsertID)

	items := make([]OrderItem, 0, len(b.Items))
	for i, item := range b.Items {
		itemRes, err := s.db.Execute(ctx, `
			INSERT INTO order_items (order_id, product_id, product_name, price, quantity, total)
			VALUES (?, ?, ?, ?, ?, ?)
		`, orderID, item.ProductID, item.ProductName, item.Price, item.Quantity, item.Total)
		if err != nil {
			log.Printf("insert order item: %v", err)
			return nil, huma.Error500InternalServerError("failed to create order item")
		}
		items = append(items, OrderItem{
			ID:              int(itemRes.LastInsertID),
			OrderItemFields: b.Items[i],
		})
	}

	return &CreateOrderOutput{Body: Order{ID: orderID, OrderFields: b.OrderFields, Items: items}}, nil
}

func (s *Service) list(ctx context.Context, in *ListOrdersInput) (*ListOrdersOutput, error) {
	countRows, err := s.db.Query(ctx, `SELECT COUNT(*) AS total FROM orders`)
	if err != nil {
		log.Printf("count orders: %v", err)
		return nil, huma.Error500InternalServerError("failed to list orders")
	}
	total := 0
	if len(countRows) > 0 {
		if v, err := countRows[0].Int("total"); err == nil {
			total = int(v)
		}
	}

	query := `
		SELECT id, user_name, email, phone, street, town_or_city, state, pincode, notes, delivery_region, delivery_location, total
		FROM orders
		ORDER BY id
	`
	queryArgs := []any{}
	if in.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		queryArgs = append(queryArgs, in.Limit, in.Offset)
	}

	rows, err := s.db.Query(ctx, query, queryArgs...)
	if err != nil {
		log.Printf("list orders: %v", err)
		return nil, huma.Error500InternalServerError("failed to list orders")
	}

	var limitPtr, offsetPtr *int
	if in.Limit > 0 {
		limitPtr = &in.Limit
		offsetPtr = &in.Offset
	}

	items := make([]Order, 0, len(rows))
	for _, row := range rows {
		items = append(items, rowToOrder(row))
	}
	return &ListOrdersOutput{Body: ListOrdersResponse{
		Items:  items,
		Total:  total,
		Limit:  limitPtr,
		Offset: offsetPtr,
	}}, nil
}

func (s *Service) get(ctx context.Context, in *GetOrderInput) (*GetOrderOutput, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_name, email, phone, street, town_or_city, state, pincode, notes, delivery_region, delivery_location, total
		FROM orders WHERE id = ?
	`, in.ID)
	if err != nil {
		log.Printf("get order %d: %v", in.ID, err)
		return nil, huma.Error500InternalServerError("failed to get order")
	}

	if len(rows) == 0 {
		return nil, huma.Error404NotFound("order not found")
	}

	order := rowToOrder(rows[0])

	itemRows, err := s.db.Query(ctx, `
		SELECT id, product_id, product_name, price, quantity, total
		FROM order_items WHERE order_id = ?
		ORDER BY id
	`, in.ID)
	if err != nil {
		log.Printf("get order items %d: %v", in.ID, err)
		return nil, huma.Error500InternalServerError("failed to get order items")
	}

	order.Items = make([]OrderItem, 0, len(itemRows))
	for _, row := range itemRows {
		order.Items = append(order.Items, rowToOrderItem(row))
	}

	return &GetOrderOutput{Body: order}, nil
}

func (s *Service) update(ctx context.Context, in *UpdateOrderInput) (*UpdateOrderOutput, error) {
	b := in.Body
	res, err := s.db.Execute(ctx, `
		UPDATE orders
		SET user_name = ?, email = ?, phone = ?, street = ?, town_or_city = ?, state = ?, pincode = ?, notes = ?, delivery_region = ?, delivery_location = ?, total = ?
		WHERE id = ?
	`, b.UserName, b.Email, b.Phone, b.Street, b.TownOrCity, b.State, b.Pincode, b.Notes, b.DeliveryRegion, b.DeliveryLocation, b.Total, in.ID)
	if err != nil {
		log.Printf("update order %d: %v", in.ID, err)
		return nil, huma.Error500InternalServerError("failed to update order")
	}

	if res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("order not found")
	}

	_, err = s.db.Execute(ctx, `DELETE FROM order_items WHERE order_id = ?`, in.ID)
	if err != nil {
		log.Printf("delete order items %d: %v", in.ID, err)
		return nil, huma.Error500InternalServerError("failed to update order items")
	}

	items := make([]OrderItem, 0, len(b.Items))
	for i, item := range b.Items {
		itemRes, err := s.db.Execute(ctx, `
			INSERT INTO order_items (order_id, product_id, product_name, price, quantity, total)
			VALUES (?, ?, ?, ?, ?, ?)
		`, in.ID, item.ProductID, item.ProductName, item.Price, item.Quantity, item.Total)
		if err != nil {
			log.Printf("insert order item: %v", err)
			return nil, huma.Error500InternalServerError("failed to update order items")
		}
		items = append(items, OrderItem{
			ID:              int(itemRes.LastInsertID),
			OrderItemFields: b.Items[i],
		})
	}

	return &UpdateOrderOutput{Body: Order{ID: in.ID, OrderFields: b.OrderFields, Items: items}}, nil
}

func (s *Service) delete(ctx context.Context, in *DeleteOrderInput) (*struct{}, error) {
	_, err := s.db.Execute(ctx, `DELETE FROM order_items WHERE order_id = ?`, in.ID)
	if err != nil {
		log.Printf("delete order items %d: %v", in.ID, err)
		return nil, huma.Error500InternalServerError("failed to delete order")
	}

	res, err := s.db.Execute(ctx, `DELETE FROM orders WHERE id = ?`, in.ID)
	if err != nil {
		log.Printf("delete order %d: %v", in.ID, err)
		return nil, huma.Error500InternalServerError("failed to delete order")
	}
	if res.RowsAffected == 0 {
		return nil, huma.Error404NotFound("order not found")
	}
	return nil, nil
}

func rowToOrder(row database.Row) Order {
	id, _ := row.Int("id")
	userName, _ := row.String("user_name")
	email, _ := row.String("email")
	phone, _ := row.String("phone")
	street, _ := row.String("street")
	townOrCity, _ := row.String("town_or_city")
	state, _ := row.String("state")
	pincode, _ := row.String("pincode")
	notes, _ := row.NullableString("notes")
	deliveryRegion, _ := row.String("delivery_region")
	deliveryLocation, _ := row.String("delivery_location")
	total, _ := row.Float("total")
	return Order{
		ID: int(id),
		OrderFields: OrderFields{
			UserName:         userName,
			Email:            email,
			Phone:            phone,
			Street:           street,
			TownOrCity:       townOrCity,
			State:            state,
			Pincode:          pincode,
			Notes:            notes,
			DeliveryRegion:   deliveryRegion,
			DeliveryLocation: deliveryLocation,
			Total:            total,
		},
	}
}

func rowToOrderItem(row database.Row) OrderItem {
	id, _ := row.Int("id")
	productID, _ := row.Int("product_id")
	productName, _ := row.String("product_name")
	price, _ := row.Float("price")
	quantity, _ := row.Int("quantity")
	total, _ := row.Float("total")
	return OrderItem{
		ID: int(id),
		OrderItemFields: OrderItemFields{
			ProductID:   int(productID),
			ProductName: productName,
			Price:       price,
			Quantity:    int(quantity),
			Total:       total,
		},
	}
}
