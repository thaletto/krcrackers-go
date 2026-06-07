-- +goose Up
CREATE TABLE IF NOT EXISTS orders (
    id INTEGER PRIMARY KEY,
    user_name TEXT NOT NULL,
    email TEXT NOT NULL,
    phone TEXT NOT NULL,
    street TEXT NOT NULL,
    town_or_city TEXT NOT NULL,
    state TEXT NOT NULL,
    pincode TEXT NOT NULL,
    notes TEXT,
    delivery_region TEXT NOT NULL,
    delivery_location TEXT NOT NULL,
    total REAL NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS order_items (
    id INTEGER PRIMARY KEY,
    order_id INTEGER NOT NULL REFERENCES orders(id),
    product_id INTEGER NOT NULL REFERENCES products(id),
    product_name TEXT NOT NULL,
    price REAL NOT NULL,
    quantity INTEGER NOT NULL,
    total REAL NOT NULL
);

-- +goose Down
DROP TABLE order_items;
DROP TABLE orders;
