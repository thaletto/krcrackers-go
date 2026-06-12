-- +goose Up
CREATE TABLE IF NOT EXISTS invoice_counters (
  id INTEGER PRIMARY KEY,
  current_number INTEGER NOT NULL DEFAULT 0
);

-- +goose Down
DROP TABLE invoice_counters;
