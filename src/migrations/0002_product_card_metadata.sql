-- +goose Up
ALTER TABLE products ADD COLUMN rating REAL;
ALTER TABLE products ADD COLUMN delivery TEXT;

-- +goose Down
ALTER TABLE products DROP COLUMN delivery;
ALTER TABLE products DROP COLUMN rating;
