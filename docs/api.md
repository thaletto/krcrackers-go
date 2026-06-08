# API

All endpoints return JSON. Errors use a simple `{"error": "message"}` format.

| Method | Path | Body | Returns |
|---|---|---|---|
| `POST` | `/products` | `ProductInput` | `201` + `Product` |
| `GET` | `/products` | None | `200` + `ListProductsResponse` |
| `GET` | `/products/{id}` | None | `200` + `Product`, `404` if missing |
| `PUT` | `/products/{id}` | `ProductInput` | `200` + `Product`, `404` if missing |
| `DELETE` | `/products/{id}` | None | `204`, `404` if missing |
| `POST` | `/orders` | `OrderInput` | `201` + `Order` |
| `GET` | `/orders` | None | `200` + `ListOrdersResponse` |
| `GET` | `/orders/{id}` | None | `200` + `Order`, `404` if missing |
| `PUT` | `/orders/{id}` | `OrderInput` | `200` + `Order`, `404` if missing |
| `DELETE` | `/orders/{id}` | None | `204`, `404` if missing |
| `GET` | `/health` | None | `200` + `{"status":200,"message":"ok"}` |

## Schema

`ProductInput` (request body): `name`, `price`, `category`, `comparePrice` are required; `brand`, `description`, `image` are optional and nullable.

```json
{
  "name": "Sneaker",
  "price": 99,
  "category": "footwear",
  "comparePrice": 129,
  "brand": "Acme",
  "description": "Shoes",
  "image": "/s.png"
}
```

`Product` (response) adds the server-assigned `id`.

## Pagination

`GET /products` and `GET /orders` accept optional `limit` and `offset` query params. Omitting `limit` returns all rows. The response includes the total count and echoes back the params (omitted when not provided).

## Validation

Required fields are validated on create/update. Missing or invalid fields return `422`:

```json
{
  "error": "name is required"
}
```
