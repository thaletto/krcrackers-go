-- +goose Up
CREATE TABLE IF NOT EXISTS products (
    id INTEGER PRIMARY KEY,
    name TEXT,
    price REAL,
    brand TEXT,
    description TEXT,
    category TEXT,
    image TEXT,
    compare_price REAL
);

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
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status TEXT NOT NULL DEFAULT 'pending',
    user_id INTEGER REFERENCES users(id),
    payment_screenshot_url TEXT DEFAULT '',
    payment_reference TEXT DEFAULT '',
    verified_at DATETIME,
    invoice_number TEXT DEFAULT ''
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

CREATE VIRTUAL TABLE products_fts USING fts5(name, description, content='products', content_rowid='id');

INSERT INTO products_fts(products_fts) VALUES('rebuild');

-- +goose StatementBegin
CREATE TRIGGER products_fts_ai AFTER INSERT ON products BEGIN
    INSERT INTO products_fts(rowid, name, description) VALUES (new.id, new.name, COALESCE(new.description, ''));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER products_fts_ad AFTER DELETE ON products BEGIN
    INSERT INTO products_fts(products_fts, rowid, name, description) VALUES ('delete', old.id, old.name, COALESCE(old.description, ''));
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER products_fts_au AFTER UPDATE ON products BEGIN
    INSERT INTO products_fts(products_fts, rowid, name, description) VALUES ('delete', old.id, old.name, COALESCE(old.description, ''));
    INSERT INTO products_fts(rowid, name, description) VALUES (new.id, new.name, COALESCE(new.description, ''));
END;
-- +goose StatementEnd

CREATE TABLE IF NOT EXISTS users (
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

CREATE TABLE IF NOT EXISTS customer_addresses (
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

CREATE TABLE IF NOT EXISTS refresh_tokens (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id),
  token TEXT NOT NULL,
  expires_at DATETIME NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TRIGGER IF EXISTS products_fts_au;
DROP TRIGGER IF EXISTS products_fts_ad;
DROP TRIGGER IF EXISTS products_fts_ai;
DROP TABLE IF EXISTS products_fts;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS customer_addresses;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS products;