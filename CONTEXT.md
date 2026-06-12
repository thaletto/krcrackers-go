# Domain Glossary

## Core Entities

- **Product** — An item available for purchase. Has name, price, description, brand,
  category, image URL, and compare price. Single image for MVP.

- **Customer** — A registered user who can browse products, place orders, and manage
  addresses. Identified by email or Google OAuth.

- **Order** — A customer's purchase request. Contains customer snapshot (name, email,
  phone), shipping address snapshot, order items, total, status, payment screenshot,
  and optional payment reference.

- **Order Item** — A line item within an order. References a product by ID and snapshots
  the product name, price, quantity, and line total at time of purchase.

- **Customer Address** — A saved shipping location belonging to a customer. Has label
  ("Home", "Office"), full address fields, and is_default flag. Orders snapshot the
  selected address at time of purchase.

- **Payment Screenshot** — An image uploaded by the customer as proof of bank transfer /
  UPI payment. Stored in R2. Reviewed by admin to confirm payment.

## Statuses

- **Order Status** — The lifecycle state of an order:
  - `pending` — Placed, awaiting payment verification
  - `confirmed` — Admin verified payment
  - `shipped` — Order dispatched
  - `delivered` — Order received by customer
  - `cancelled` — Order cancelled (payment failed, timeout, or customer request)

- **User Role** — Either `customer` (default) or `admin`. Controls access to
  admin routes.

## Services

- **Meilisearch** — Search engine providing typo-tolerant full-text search, filtering,
  and sorting across the product catalog.

- **Cloudflare R2** — Object storage for payment screenshots. S3-compatible, no
  egress fees.

- **WhatsApp Cloud API** — Meta's official WhatsApp Business API. Used for order
  status notifications and (later) promotions.
