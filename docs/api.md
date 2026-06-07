# API

All endpoints under `/products` use JSON. Errors follow [RFC 7807](https://www.rfc-editor.org/rfc/rfc7807) (`application/problem+json`).

| Method | Path | Body | Returns |
|---|---|---|---|
| `POST` | `/products` | `ProductInput` | `201` + `Product` |
| `GET` | `/products` | None | `200` + `[]Product` |
| `GET` | `/products/{id}` | None | `200` + `Product`, `404` if missing |
| `PUT` | `/products/{id}` | `ProductInput` | `200` + `Product`, `404` if missing |
| `DELETE` | `/products/{id}` | None | `204`, `404` if missing |
| `GET` | `/health` | None | `204` |
| `GET` | `/openapi.json` | None | OpenAPI 3.1 spec |
| `GET` | `/docs` | None | Stoplight Elements UI |

## Schema

`ProductInput` (request body): `name`, `price`, `category`, `comparePrice` are required; `brand`, `description`, `image` are optional and nullable.

```json
{
  "name": "Sneaker",          // required, minLength 1
  "price": 99,                // required, >= 0
  "category": "footwear",     // required, minLength 1
  "comparePrice": 129,        // required, >= 0
  "brand": "Acme",            // optional, nullable
  "description": "Shoes",     // optional, nullable
  "image": "/s.png"           // optional, nullable
}
```

`Product` (response) adds the server-assigned `id`.

## Validation

Huma validates path params, body, and required fields automatically and returns `422` with a per-field error list:

```json
{
  "title": "Unprocessable Entity",
  "status": 422,
  "detail": "validation failed",
  "errors": [
    {"location": "body.price", "message": "expected number >= 0", "value": -5}
  ]
}
```
