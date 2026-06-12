-- +goose Up
ALTER TABLE orders ADD COLUMN status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE orders ADD COLUMN user_id INTEGER REFERENCES users(id);
ALTER TABLE orders ADD COLUMN payment_screenshot_url TEXT DEFAULT '';
ALTER TABLE orders ADD COLUMN payment_reference TEXT DEFAULT '';
ALTER TABLE orders ADD COLUMN verified_at DATETIME;

-- +goose Down
ALTER TABLE orders DROP COLUMN verified_at;
ALTER TABLE orders DROP COLUMN payment_reference;
ALTER TABLE orders DROP COLUMN payment_screenshot_url;
ALTER TABLE orders DROP COLUMN user_id;
ALTER TABLE orders DROP COLUMN status;
