-- +goose Up
CREATE VIRTUAL TABLE products_fts USING fts5(name, description, content='products', content_rowid='id');

-- +goose Down
DROP TABLE IF EXISTS products_fts;
