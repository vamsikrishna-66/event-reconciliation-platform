CREATE TABLE IF NOT EXISTS reconciliation_events (
    event_id text PRIMARY KEY,
    order_id text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('order_created', 'payment_captured', 'refund_issued')),
    amount_cents bigint NOT NULL CHECK (amount_cents > 0)
);

CREATE INDEX IF NOT EXISTS reconciliation_events_order_idx
    ON reconciliation_events (order_id);

CREATE OR REPLACE VIEW order_reconciliation AS
WITH totals AS (
    SELECT order_id,
           count(*) FILTER (WHERE kind = 'order_created') AS order_events,
           coalesce(sum(amount_cents) FILTER (WHERE kind = 'order_created'), 0) AS expected_cents,
           coalesce(sum(amount_cents) FILTER (WHERE kind = 'payment_captured'), 0) AS paid_cents,
           coalesce(sum(amount_cents) FILTER (WHERE kind = 'refund_issued'), 0) AS refunded_cents
    FROM reconciliation_events GROUP BY order_id
)
SELECT *, paid_cents - refunded_cents AS net_cents,
       CASE
           WHEN order_events = 0 THEN 'missing_order'
           WHEN order_events > 1 THEN 'multiple_orders'
           WHEN refunded_cents > paid_cents THEN 'refund_exceeds_payment'
           WHEN paid_cents < expected_cents THEN 'underpaid'
           WHEN paid_cents > expected_cents THEN 'overpaid'
           ELSE 'reconciled'
       END AS status
FROM totals;
