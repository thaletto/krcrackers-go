-- +goose Up
CREATE UNIQUE INDEX IF NOT EXISTS users_google_provider_id_unique
ON users(auth_provider_id)
WHERE auth_provider = 'google' AND auth_provider_id <> '';

-- +goose Down
DROP INDEX IF EXISTS users_google_provider_id_unique;
