-- Link orders to the account that placed them.
--
-- Guest checkout never recorded user_id, so every order placed through the
-- storefront was invisible in /orders/my even when the shopper was signed in;
-- only the admin dashboard, which lists orders unfiltered, showed them. The
-- request handler now stamps user_id from the session, but rows written before
-- that fix stay NULL forever, so recover them by email - the only identifier
-- linking an old order to an account.
--
-- Matching is case-insensitive because account emails are not normalised on
-- the way in. An order whose email matches no account keeps its NULL user_id
-- and remains a guest order.
-- +goose Up
UPDATE orders
SET user_id = (SELECT users.id FROM users WHERE lower(users.email) = lower(orders.email))
WHERE user_id IS NULL
  AND EXISTS (SELECT 1 FROM users WHERE lower(users.email) = lower(orders.email));

-- +goose Down
-- One-way by nature: orders that were always guest orders are
-- indistinguishable from the ones this migration just claimed, so unlinking
-- them all would be wrong and unlinking none leaves the rollback as a no-op.
-- Choose the no-op.
SELECT 1;
