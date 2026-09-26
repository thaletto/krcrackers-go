-- Google account linking.
--
-- auth_provider/auth_provider_id had room for exactly one provider per row, so
-- an account created with a password could never also sign in with Google:
-- overwriting the pair would strand the password, and leaving it alone made the
-- Google subject unrepresentable. google_id is independent of auth_provider
-- (which keeps meaning "how this account was created"), so a single row can now
-- carry both a bcrypt hash and a Google subject.
--
-- auth_provider_id is left in place for rows written before this migration but
-- is no longer read; google_id is the single source of truth for Google links.
-- +goose Up
-- Comments inside a section are stripped before the statements are split, so
-- they are safe here (they used to cause the following statement to be dropped).
ALTER TABLE users ADD COLUMN google_id TEXT;

-- Backfill accounts that were created via Google before this column existed.
UPDATE users
SET google_id = auth_provider_id
WHERE auth_provider = 'google' AND auth_provider_id IS NOT NULL AND auth_provider_id <> '';

-- One Google subject per account. NULL and '' are excluded so unlinked rows
-- and password accounts can share the column.
CREATE UNIQUE INDEX IF NOT EXISTS users_google_id_unique
ON users(google_id)
WHERE google_id IS NOT NULL AND google_id <> '';

-- +goose Down
DROP INDEX IF EXISTS users_google_id_unique;

-- Restore the Google subject to the legacy column before dropping it, so a
-- rollback does not lose which accounts had Google sign-in enabled.
UPDATE users SET auth_provider_id = google_id WHERE google_id IS NOT NULL;

ALTER TABLE users DROP COLUMN google_id;
