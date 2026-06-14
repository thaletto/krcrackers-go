-- +goose Up
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

-- +goose Down
DROP TRIGGER IF EXISTS products_fts_au;
DROP TRIGGER IF EXISTS products_fts_ad;
DROP TRIGGER IF EXISTS products_fts_ai;