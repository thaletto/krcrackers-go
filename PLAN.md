# E-Commerce Backend Implementation Plan

## Architecture Overview

```
kr-crackers-api/
├── services/
│   ├── auth/            ← NEW: Google OAuth, email/password, JWT
│   ├── customers/       ← NEW: profile, addresses
│   ├── products/        ← EXPANDED: Meilisearch integration
│   ├── orders/          ← EXPANDED: checkout, status management
│   ├── uploads/         ← NEW: R2 file uploads
│   ├── invoices/        ← NEW: PDF generation
│   └── notifications/   ← NEW: WhatsApp Cloud API
├── server/
│   └── middleware.go    ← NEW: Auth, admin role checks
├── database/
│   └── (existing)       ← EXPANDED: new migrations
└── main.go              ← EXPANDED: wire new services
```

## Decisions

| Decision | Choice |
|----------|--------|
| Target user | Customer-facing first |
| Order lifecycle | Pending → Confirmed → Shipped → Delivered / Cancelled |
| Payment proof | Screenshot upload (R2) |
| File storage | Cloudflare R2 |
| Auth | Google OAuth + email/password fallback |
| Auth tokens | JWT + refresh token pair (HTTP-only cookies) |
| Cart | Client-side localStorage |
| Search | Meilisearch (typo-tolerance, filtering, sorting) |
| Inventory | Skipped |
| Customer addresses | Separate table, order snapshots address |
| Order display | Status badge only (MVP) |
| Code structure | Sub-packages per domain |
| Admin | Same binary, `/api/admin/*` routes, role check middleware |
| Product images | Single image (MVP) |
| Reviews | Skipped |
| Notifications | WhatsApp Cloud API |
| Invoice | On-demand PDF generation |

---

## Phase 0: Foundation

### 0.1 — Meilisearch Setup

- Add Meilisearch to `docker-compose.yml` (or `.env` config for dev)
- Create `services/search/` package — Meilisearch client wrapper
- Create migration: `0003_products_fts.sql` — FTS5 virtual table as fallback
- Index configuration: filterable (category, brand, price), sortable (price, created_at)
- Product create/update/delete → sync to Meilisearch index

### 0.2 — R2 Setup

- Add R2 bucket config to `wrangler.toml`
- Create `services/uploads/` package — R2 client wrapper
- `POST /uploads` → upload file, return URL
- `DELETE /uploads/{key}` → remove file
- Configure CORS, public access for screenshot viewing

### 0.3 — Database Schema Expansion

New migrations:

```sql
-- 0004_users.sql
CREATE TABLE users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  email TEXT UNIQUE NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  phone TEXT DEFAULT '',
  avatar_url TEXT DEFAULT '',
  auth_provider TEXT NOT NULL DEFAULT 'email',
  auth_provider_id TEXT DEFAULT '',
  password_hash TEXT DEFAULT '',
  role TEXT NOT NULL DEFAULT 'customer',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 0005_customer_addresses.sql
CREATE TABLE customer_addresses (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id),
  label TEXT NOT NULL DEFAULT 'Home',
  street TEXT NOT NULL,
  city TEXT NOT NULL,
  state TEXT NOT NULL,
  pincode TEXT NOT NULL,
  country TEXT NOT NULL DEFAULT 'India',
  is_default BOOLEAN DEFAULT FALSE,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 0006_orders_v2.sql
ALTER TABLE orders ADD COLUMN user_id INTEGER REFERENCES users(id);
ALTER TABLE orders ADD COLUMN payment_screenshot_url TEXT DEFAULT '';
ALTER TABLE orders ADD COLUMN payment_reference TEXT DEFAULT '';
ALTER TABLE orders ADD COLUMN verified_at DATETIME;

-- 0007_refresh_tokens.sql
CREATE TABLE refresh_tokens (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id),
  token TEXT NOT NULL,
  expires_at DATETIME NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 0008_invoice_counters.sql
CREATE TABLE invoice_counters (
  id INTEGER PRIMARY KEY,
  current_number INTEGER NOT NULL DEFAULT 0
);
```

### 0.4 — Environment Variables

New env vars:

```sh
MEILISEARCH_URL=http://localhost:7700
MEILISEARCH_API_KEY=...
R2_ACCOUNT_ID=...
R2_ACCESS_KEY_ID=...
R2_SECRET_ACCESS_KEY=...
R2_BUCKET_NAME=...
GOOGLE_OAUTH_CLIENT_ID=...
GOOGLE_OAUTH_CLIENT_SECRET=...
JWT_SECRET=...
WHATSAPP_API_TOKEN=...
WHATSAPP_PHONE_NUMBER_ID=...
WHATSAPP_FROM_NUMBER=...
```

---

## Phase 1: Auth & Customer Profile

### 1.1 — Auth Service (`services/auth/`)

- `POST /auth/register` — email + password registration
  - Hash password (bcrypt)
  - Send verification email (placeholder for now)
  - Return JWT + refresh token
- `POST /auth/login` — email + password login
  - Validate credentials
  - Return JWT + refresh token
- `GET /auth/google` — Google OAuth redirect
- `GET /auth/google/callback` — Google OAuth callback
  - Find or create user by google_id
  - Return JWT + refresh token
- `POST /auth/refresh` — refresh access token
- `POST /auth/logout` — revoke refresh token
- `GET /auth/me` — get current user

Token implementation:

- Access token: 15-minute expiry, stored in HTTP-only cookie
- Refresh token: 7-day expiry, stored in HTTP-only cookie + refresh_tokens table
- Role claim in JWT: `customer` or `admin`

### 1.2 — Middleware (`server/middleware.go`)

- `WithAuth` — extract user from JWT, inject into context
- `WithAdmin` — check user role is `admin`, return 403 if not
- `WithOptionalAuth` — extract user if present, continue if not

### 1.3 — Customers Service (`services/customers/`)

- `GET /customers/profile` — get current user's profile
- `PUT /customers/profile` — update profile (name, phone)
- `GET /customers/addresses` — list saved addresses
- `POST /customers/addresses` — add address
- `PUT /customers/addresses/{id}` — update address
- `DELETE /customers/addresses/{id}` — delete address
- `PUT /customers/addresses/{id}/default` — set as default

---

## Phase 2: Product Catalog (Expand Existing)

### 2.1 — Update Products Service

- Product model: add `brand` field (already exists in schema, verify)
- Image handling: single image upload via `/uploads`, URL stored on product
- Meilisearch sync: on create/update/delete, index to Meilisearch

### 2.2 — Enhanced Product Endpoints

- `GET /products` — add query params: `q` (search), `category`, `brand`, `min_price`, `max_price`, `sort` (price_asc, price_desc, newest)
- Search routed through Meilisearch when `q` is present
- Filtering via Meilisearch facets

---

## Phase 3: Orders & Checkout

### 3.1 — Checkout Flow

- `POST /orders/checkout` — place order
  - Request body: `{ items: [{product_id, quantity}], address_id, payment_screenshot (file), payment_reference? }`
  - Validate items exist, calculate totals
  - Snapshot customer name, email, phone from profile
  - Snapshot address from customer_addresses
  - Create order (status: pending) + order_items in transaction
  - Upload screenshot to R2, store URL on order
  - Send WhatsApp: "Order placed"
  - Return order

### 3.2 — Order Management (Customer)

- `GET /orders` — list current user's orders (with pagination)
- `GET /orders/{id}` — get order detail (own orders only)
- `DELETE /orders/{id}` — cancel order (only if status is `pending`)

### 3.3 — Order Management (Admin)

- `GET /admin/orders` — list all orders (filterable by status)
- `GET /admin/orders/{id}` — get order detail with payment screenshot URL
- `PATCH /admin/orders/{id}/status` — update status
  - `pending` → `confirmed` (payment verified)
  - `confirmed` → `shipped`
  - `shipped` → `delivered`
  - any → `cancelled`
  - Send WhatsApp notification on each transition
- `GET /admin/orders/{id}/screenshot` — get pre-signed R2 URL for screenshot

### 3.4 — Admin Product Management

- `POST /admin/products` — create product (with image upload)
- `PUT /admin/products/{id}` — update product
- `DELETE /admin/products/{id}` — delete product

---

## Phase 4: Notifications

### 4.1 — WhatsApp Service (`services/notifications/`)

- WhatsApp Cloud API client wrapper
- Template messages:
  - `order_placed` — "Your order #{id} is confirmed! We'll verify your payment shortly."
  - `payment_confirmed` — "Payment confirmed for order #{id}! Your order is being prepared."
  - `order_shipped` — "Your order #{id} is on the way!"
  - `order_delivered` — "Your order #{id} has been delivered. Thank you!"
  - `order_cancelled` — "Your order #{id} has been cancelled."
- Integration points: order status update handler calls notification service

### 4.2 — Notification Service

- `POST /admin/notifications/send` — manual notification (for future promotions)
- Queue/async sending (avoid blocking order status updates)

---

## Phase 5: Invoice Generation

### 5.1 — Invoice Service (`services/invoices/`)

- PDF generation using `github.com/jung-kurt/gofpdf` or `github.com/signintech/gopdf`
- Invoice template:
  - Header: Business name, address, GST number (if applicable)
  - Invoice number (sequential: INV-0001, INV-0002, ...)
  - Date, order reference
  - Customer details (name, email, phone, address)
  - Itemized table: product, qty, unit price, line total
  - Subtotal, taxes (if applicable), total
- `GET /orders/{id}/invoice` — generate and return PDF (customer, own orders)
- `GET /admin/orders/{id}/invoice` — generate and return PDF (admin, any order)

### 5.2 — Invoice Storage (Optional)

- Cache generated invoices in R2 to avoid regenerating on every request
- Invalidate on order update

---

## Phase 6: Admin Dashboard API

### 6.1 — Admin Endpoints

- `GET /admin/dashboard` — summary stats
  - Total orders, pending orders, revenue this month, new customers
- `GET /admin/customers` — list all customers (read-only)
- `GET /admin/customers/{id}` — customer detail with order history

---

## Implementation Order (Recommended)

| Week | Phase | Deliverable |
|------|-------|-------------|
| 1 | 0 | Meilisearch, R2, schema migrations, env vars |
| 2 | 1 | Auth service, middleware, customer profile & addresses |
| 3 | 2 | Enhanced products with search, filtering, sorting |
| 4 | 3 | Checkout flow, order management, admin endpoints |
| 5 | 4 | WhatsApp notifications |
| 6 | 5 | PDF invoice generation |
| 7 | 6 | Admin dashboard stats |
