# KR Crackers API v1.0

E-commerce backend for KR Crackers with order management, product catalog, and admin dashboard.



## Authentication

Cookie-based JWT (`access_token` in HttpOnly cookie). `401` = Unauthorized, `403` = Forbidden.

Endpoints marked **Auth** require the `access_token` cookie.



## Common Patterns

### Pagination query params
`limit` (int), `offset` (int) - optional. `0` = omitted.

### `{id}` path param
Integer resource ID. `400` = Bad Request, `404` = Not Found.

### Error responses
`400`, `401`, `403`, `404`, `422`, `500` return `server.ErrorResponse`:
```json
{ "error": "string" }
```

### `StatusResponse`
```json
{ "status": "string" }
```



## Admin (`/admin/*`)
**Auth + admin role required.**

| Method | Endpoint | Summary | Body | Response |
|--|-|||-|
| `GET` | `/admin/dashboard` | Dashboard stats | - | `orders.DashboardStats` |
| `GET` | `/admin/orders` | List all orders | - | `orders.ListOrdersResponse` |
| `GET` | `/admin/orders/{id}` | Get order details | - | `orders.Order` |
| `PATCH` | `/admin/orders/{id}/status` | Update order status | `{ "status": string }` | `orders.Order` |
| `POST` | `/admin/products` | Create product | `products.ProductInput` | `products.Product` |
| `PUT` | `/admin/products/{id}` | Update product | `products.ProductInput` | `products.Product` |
| `DELETE` | `/admin/products/{id}` | Delete product | - | `204` |
| `GET` | `/admin/invoices/{id}` | Download invoice PDF | - | `application/pdf` |



## Auth (`/auth/*`)
**Public.**

| Method | Endpoint | Summary | Body | Response |
|--|-|||-|
| `POST` | `/auth/register` | Register | `{ email, password, name, phone }` | `auth.authResponse` |
| `POST` | `/auth/login` | Log in | `{ email, password }` | `auth.authResponse` |
| `POST` | `/auth/google` | Google login | `{ id_token }` | `auth.authResponse` |
| `POST` | `/auth/refresh` | Refresh token | - | `auth.authResponse` |
| `POST` | `/auth/logout` | Log out | - | `204` |
| `GET` | `/auth/me` | Current user | - | `auth.User` |



## Customers (`/customers/*`)
**Auth required.**

| Method | Endpoint | Summary | Body | Response |
|--|-|||-|
| `GET` | `/customers/profile` | Get profile | - | `auth.User` |
| `PUT` | `/customers/profile` | Update profile | `{ name, phone }` | `auth.User` |
| `GET` | `/customers/addresses` | List addresses | - | `customers.ListAddressesResponse` |
| `POST` | `/customers/addresses` | Create address | `customers.AddressInput` | `customers.Address` |
| `PUT` | `/customers/addresses/{id}` | Update address | `customers.AddressInput` | `customers.Address` |
| `DELETE` | `/customers/addresses/{id}` | Delete address | - | `204` |
| `PUT` | `/customers/addresses/{id}/default` | Set default | - | `server.StatusResponse` |



## Orders (`/orders/*`)

| Method | Endpoint | Auth | Summary | Body | Response |
|--|-||||-|
| `GET` | `/orders` | No | List orders | - | `orders.ListOrdersResponse` |
| `POST` | `/orders` | No | Create order | `orders.OrderInput` | `orders.Order` |
| `GET` | `/orders/{id}` | No | Get order | - | `orders.Order` |
| `PUT` | `/orders/{id}` | No | Update order | `orders.OrderInput` | `orders.Order` |
| `DELETE` | `/orders/{id}` | No | Delete order | - | `204` |
| `POST` | `/orders/checkout` | **Yes** | Checkout | `multipart/form-data`¹ | `orders.Order` |
| `GET` | `/orders/my` | **Yes** | List my orders | - | `orders.ListOrdersResponse` |
| `GET` | `/orders/my/{id}` | **Yes** | Get my order | - | `orders.Order` |
| `DELETE` | `/orders/my/{id}` | **Yes** | Cancel my order | - | `orders.Order` |

¹ `address_id` (int), `items` (JSON string), `payment_screenshot` (binary), `payment_reference` (string)



## Invoices (`/invoices/*`)
**Auth required.**

| Method | Endpoint | Summary | Response |
|--|-||-|
| `GET` | `/invoices/{id}` | Download invoice PDF | `application/pdf` |



## Products (`/products/*`)
**Public.**

| Method | Endpoint | Summary | Query Params | Response |
|--|-||-|-|
| `GET` | `/products` | List products | `q`, `category`, `brand`, `min_price`, `max_price`, `sort`, `limit`, `offset` | `products.ListProductsResponse` |
| `GET` | `/products/{id}` | Get product | - | `products.Product` |



## Schemas

### `auth.User`
```json
{
  "id": 0, "email": "", "name": "", "phone": "", "role": "",
  "authProvider": "", "avatarUrl": "", "createdAt": "", "updatedAt": ""
}
```

### `auth.authResponse`
```json
{ "user": {} }
```

### `customers.Address`
```json
{
  "id": 0, "userId": 0, "label": "", "street": "", "city": "",
  "state": "", "country": "", "pincode": "", "isDefault": false,
  "createdAt": "", "updatedAt": ""
}
```

### `customers.AddressInput`
```json
{
  "label": "", "street": "", "city": "", "state": "",
  "country": "", "pincode": "", "isDefault": false
}
```

### `customers.ListAddressesResponse`
```json
{ "items": [customers.Address], "total": 0 }
```

### `orders.DashboardStats`
```json
{ "totalOrders": 0, "pendingOrders": 0, "revenueMonth": 0, "newCustomers": 0 }
```

### `orders.Order`
```json
{
  "id": 0, "userId": 0, "userName": "", "email": "", "phone": "",
  "street": "", "townOrCity": "", "state": "", "pincode": "",
  "deliveryRegion": "", "deliveryLocation": "", "status": "pending",
  "total": 0, "notes": "", "paymentReference": "",
  "paymentScreenshotUrl": "", "createdAt": "",
  "items": [orders.OrderItem]
}
```

### `orders.OrderItem`
```json
{
  "id": 0, "productId": 0, "productName": "", "quantity": 0,
  "price": 0, "total": 0
}
```

### `orders.OrderInput`
Same as `orders.Order` except no `id`, `items` is `orders.OrderItemFields[]` (no `id` field).

### `orders.OrderStatus`
`"pending" | "confirmed" | "shipped" | "delivered" | "cancelled"`

### `orders.ListOrdersResponse`
```json
{ "items": [orders.Order], "total": 0, "limit": 0, "offset": 0 }
```

### `products.Product`
```json
{
  "id": 0, "name": "", "description": "", "price": 0,
  "comparePrice": 0, "category": "", "brand": "", "image": "",
  "rating": 4.8, "delivery": "Delivery in 2 days"
}
```

### `products.ProductInput`
Same as `products.Product` except no `id`.

### `products.ListProductsResponse`
```json
{ "items": [products.Product], "total": 0, "limit": 0, "offset": 0 }
```

### `server.ErrorResponse`
```json
{ "error": "string" }
```

### `server.StatusResponse`
```json
{ "status": "string" }
```
