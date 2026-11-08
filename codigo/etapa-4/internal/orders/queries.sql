-- name: ActivePriceTable :one
SELECT version, rules FROM price_tables WHERE status = 'active';

-- name: ListPriceTables :many
SELECT version, status, rules, created_by, created_at, approved_by, approved_at
FROM price_tables
ORDER BY version DESC
LIMIT 50;

-- Locks the version until the end of the approval transaction.
-- name: LockPriceTable :one
SELECT version, status, created_by FROM price_tables WHERE version = @version FOR UPDATE;

-- name: InsertPriceTable :one
INSERT INTO price_tables (status, rules, created_by)
VALUES ('draft', @rules, @created_by)
RETURNING version;

-- name: RetireActivePriceTable :exec
UPDATE price_tables SET status = 'retired' WHERE status = 'active';

-- name: ActivatePriceTable :execrows
UPDATE price_tables
SET status = 'active', approved_by = @approved_by, approved_at = now()
WHERE version = @version AND status = 'draft';

-- name: InsertOrder :exec
INSERT INTO orders (
	id, owner_id, file_id, material, color, quality, finishing, quantity,
	price_table_version, unit_cents, discount_cents, shipping_cents, total_cents,
	volume_mm3, bbox_mm, dfm_flags, status, quote_expires_at,
	ship_recipient, ship_street, ship_number, ship_city, ship_state, ship_postal_code
) VALUES (
	@id, @owner_id, @file_id, @material, @color, @quality, @finishing, @quantity,
	@price_table_version, @unit_cents, @discount_cents, @shipping_cents, @total_cents,
	@volume_mm3, @bbox_mm, @dfm_flags, @status, @quote_expires_at,
	@ship_recipient, @ship_street, @ship_number, @ship_city, @ship_state, @ship_postal_code
);

-- There is no customer lookup by ID alone: the owner is part of the query
-- (D01).
-- name: GetOrderForOwner :one
SELECT * FROM orders WHERE id = @id AND owner_id = @owner_id;

-- For the panel, which checks the role and the assignment in authz.
-- name: GetOrder :one
SELECT * FROM orders WHERE id = @id;

-- name: ListOrdersForOwner :many
SELECT * FROM orders WHERE owner_id = @owner_id ORDER BY created_at DESC LIMIT 100;

-- name: ListOrdersByStatus :many
SELECT * FROM orders
WHERE (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text)
ORDER BY created_at DESC
LIMIT 200;

-- name: ListOrdersAssigned :many
SELECT * FROM orders WHERE assigned_technician = @technician ORDER BY created_at DESC LIMIT 200;

-- Changes the status only from the expected one: a concurrent change makes it
-- affect no rows.
-- name: TransitionOrder :execrows
UPDATE orders SET status = @to_status, updated_at = now()
WHERE id = @id AND status = @from_status;

-- name: AssignOrder :execrows
UPDATE orders SET assigned_technician = @technician, updated_at = now()
WHERE id = @id AND status = 'awaiting_review';

-- name: InsertStatusHistory :exec
INSERT INTO order_status_history (order_id, from_status, to_status, actor)
VALUES (@order_id, @from_status, @to_status, @actor);

-- name: ListStatusHistory :many
SELECT from_status, to_status, at FROM order_status_history WHERE order_id = @order_id ORDER BY at;

-- name: InsertReview :exec
INSERT INTO reviews (order_id, technician_id, decision, report)
VALUES (@order_id, @technician_id, @decision, @report);

-- name: GetReview :one
SELECT order_id, technician_id, decision, report, created_at FROM reviews WHERE order_id = @order_id;

-- name: SetShipment :execrows
UPDATE orders SET shipment_id = @shipment_id, tracking_code = @tracking_code, updated_at = now()
WHERE id = @id AND status = 'in_production' AND shipment_id IS NULL;

-- name: InsertPayment :exec
INSERT INTO payments (charge_id, order_id, amount_cents, status)
VALUES (@charge_id, @order_id, @amount_cents, 'pending');

-- name: LivePayment :one
SELECT charge_id, order_id, amount_cents, status, created_at, updated_at
FROM payments
WHERE order_id = @order_id AND status IN ('pending', 'succeeded');

-- Locks the charge until the end of the webhook transaction.
-- name: LockPayment :one
SELECT charge_id, order_id, amount_cents, status FROM payments WHERE charge_id = @charge_id FOR UPDATE;

-- name: SucceedPayment :execrows
UPDATE payments SET status = 'succeeded', updated_at = now() WHERE charge_id = @charge_id AND status = 'pending';

-- A replayed event conflicts with the primary key and inserts nothing.
-- name: RecordPartnerEvent :execrows
INSERT INTO partner_events (event_id, partner, type) VALUES (@event_id, @partner, @type)
ON CONFLICT (event_id) DO NOTHING;
