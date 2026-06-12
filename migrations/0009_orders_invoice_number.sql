-- +goose Up
ALTER TABLE orders ADD COLUMN invoice_number TEXT DEFAULT '';

-- +goose Down
ALTER TABLE orders DROP COLUMN invoice_number;
