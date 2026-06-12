# API

All endpoints return JSON. Errors use `{"error": "message"}` format.

## Authentication

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/auth/register` | Public | Register with email/password |
| `POST` | `/auth/login` | Public | Login with email/password |
| `POST` | `/auth/google` | Public | Login with Google ID token |
| `POST` | `/auth/refresh` | Public | Refresh access token |
| `POST` | `/auth/logout` | Public | Clear tokens |
| `GET` | `/auth/me` | WithAuth | Get current user |

## Customers

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/customers/profile` | WithAuth | Get profile |
| `PUT` | `/customers/profile` | WithAuth | Update profile |
| `GET` | `/customers/addresses` | WithAuth | List addresses |
| `POST` | `/customers/addresses` | WithAuth | Create address |
| `PUT` | `/customers/addresses/{id}` | WithAuth | Update address |
| `DELETE` | `/customers/addresses/{id}` | WithAuth | Delete address |
| `PUT` | `/customers/addresses/{id}/default` | WithAuth | Set default address |

## Products

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/products` | Public | List/search products |
| `GET` | `/products/{id}` | Public | Get product |
| `POST` | `/admin/products` | Admin | Create product |
| `PUT` | `/admin/products/{id}` | Admin | Update product |
| `DELETE` | `/admin/products/{id}` | Admin | Delete product |

## Orders

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/orders` | Public | Create order (legacy) |
| `GET` | `/orders` | Public | List orders (legacy) |
| `GET` | `/orders/{id}` | Public | Get order (legacy) |
| `PUT` | `/orders/{id}` | Public | Update order (legacy) |
| `DELETE` | `/orders/{id}` | Public | Delete order (legacy) |
| `POST` | `/orders/checkout` | WithAuth | Checkout with address + screenshot |
| `GET` | `/orders/my` | WithAuth | List my orders |
| `GET` | `/orders/my/{id}` | WithAuth | Get my order |
| `DELETE` | `/orders/my/{id}` | WithAuth | Cancel my order (pending only) |
| `GET` | `/admin/orders` | Admin | List all orders (filterable) |
| `GET` | `/admin/orders/{id}` | Admin | Get order details |
| `PATCH` | `/admin/orders/{id}/status` | Admin | Update order status |
| `GET` | `/admin/dashboard` | Admin | Dashboard stats |

## Invoices

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/invoices/{id}` | WithAuth | Download PDF invoice |
| `GET` | `/admin/invoices/{id}` | Admin | Download PDF invoice (admin) |

## Health

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/health` | Public | Health check |

## Query Parameters

### GET /products

| Param | Type | Description |
|-------|------|-------------|
| `q` | string | Full-text search query |
| `category` | string | Filter by category |
| `brand` | string | Filter by brand |
| `min_price` | float | Minimum price |
| `max_price` | float | Maximum price |
| `sort` | string | `price_asc`, `price_desc`, or `newest` |
| `limit` | int | Max items (omit for all) |
| `offset` | int | Skip N items |

### GET /admin/orders

| Param | Type | Description |
|-------|------|-------------|
| `status` | string | Filter by status |
| `limit` | int | Max items |
| `offset` | int | Skip N items |

## Order Status Transitions

```
pending → confirmed → shipped → delivered
   └→ cancelled   └→ cancelled   └→ cancelled
```

## Pagination Response

```json
{
  "items": [...],
  "total": 42,
  "limit": 20,
  "offset": 0
}
```

When `limit` is omitted, `limit` and `offset` are omitted from the response.

## Checkout (multipart/form-data)

| Field | Type | Required |
|-------|------|----------|
| `address_id` | int | Yes |
| `items` | JSON string | Yes |
| `payment_screenshot` | file | No |
| `payment_reference` | string | No |

Items JSON format:
```json
[{"productId": 1, "productName": "Item", "price": 100, "quantity": 2}]
```
