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
	ship_recipient, ship_street, ship_number, ship_city, ship_state, ship_postal_code, contact_email
) VALUES (
	@id, @owner_id, @file_id, @material, @color, @quality, @finishing, @quantity,
	@price_table_version, @unit_cents, @discount_cents, @shipping_cents, @total_cents,
	@volume_mm3, @bbox_mm, @dfm_flags, @status, @quote_expires_at,
	@ship_recipient, @ship_street, @ship_number, @ship_city, @ship_state, @ship_postal_code, @contact_email
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

-- The most recent charge, whatever its status, for the order details.
-- name: LatestPayment :one
SELECT charge_id, order_id, amount_cents, status, refund_id, created_at, updated_at
FROM payments
WHERE order_id = @order_id
ORDER BY created_at DESC
LIMIT 1;

-- name: RequestRefund :execrows
UPDATE payments SET status = 'refund_pending', updated_at = now()
WHERE charge_id = @charge_id AND status = 'succeeded';

-- name: CompleteRefund :execrows
UPDATE payments SET status = 'refunded', refund_id = @refund_id, updated_at = now()
WHERE charge_id = @charge_id AND status = 'refund_pending';

-- name: FilePurged :one
SELECT (purged_at IS NOT NULL)::boolean AS purged FROM files WHERE id = @id;

-- Address change (C21). A new proposal replaces the pending one.
-- name: SupersedeAddressChanges :exec
UPDATE address_changes SET status = 'superseded', decided_at = now()
WHERE order_id = @order_id AND status = 'pending';

-- name: InsertAddressChange :exec
INSERT INTO address_changes (
	id, order_id, requested_by, ticket_id,
	new_recipient, new_street, new_number, new_city, new_state, new_postal_code,
	code_hash, expires_at, status
) VALUES (
	@id, @order_id, @requested_by, @ticket_id,
	@new_recipient, @new_street, @new_number, @new_city, @new_state, @new_postal_code,
	@code_hash, @expires_at, 'pending'
);

-- name: DecideAddressChange :exec
UPDATE address_changes SET status = @status, decided_at = now() WHERE id = @id;

-- name: PendingAddressChange :one
SELECT * FROM address_changes WHERE order_id = @order_id AND status = 'pending';

-- Locks the pending change until the end of the confirmation.
-- name: LockPendingAddressChange :one
SELECT * FROM address_changes WHERE order_id = @order_id AND status = 'pending' FOR UPDATE;

-- name: CountAddressAttempt :one
UPDATE address_changes
SET attempts = attempts + 1,
	status = CASE WHEN attempts + 1 >= @max_attempts::integer THEN 'locked' ELSE status END,
	decided_at = CASE WHEN attempts + 1 >= @max_attempts::integer THEN now() ELSE decided_at END
WHERE id = @id
RETURNING attempts, status;

-- The address only changes before the dispatch (C21).
-- name: SetAddress :execrows
UPDATE orders
SET ship_recipient = @recipient, ship_street = @street, ship_number = @number,
	ship_city = @city, ship_state = @state, ship_postal_code = @postal_code, updated_at = now()
WHERE id = @id AND shipment_id IS NULL
	AND status IN ('quoted', 'awaiting_review', 'awaiting_payment', 'paid', 'in_production');
