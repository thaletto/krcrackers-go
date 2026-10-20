-- +goose Up
-- The refresh_tokens.token column now holds a SHA-256 digest rather than the
-- token itself, so a read of this table does not hand over a week of bearer
-- credentials for every account. The index makes the lookup an index scan
-- (every refresh did a full table scan) and enforces that a digest is unique,
-- which the lookup relies on to identify exactly one row.
CREATE UNIQUE INDEX IF NOT EXISTS refresh_tokens_token_unique ON refresh_tokens (token);

-- +goose Down
DROP INDEX IF EXISTS refresh_tokens_token_unique;
