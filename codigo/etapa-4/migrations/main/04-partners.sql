-- Payment gateway and carrier (E8.2, E8.3, FC5).

-- Shipping: price in the quote (RF07), the minimum address to deliver (LGPD:
-- data minimization) and the carrier's references.
ALTER TABLE orders
	ADD COLUMN shipping_cents   bigint NOT NULL DEFAULT 0 CHECK (shipping_cents >= 0),
	ADD COLUMN ship_recipient   text NOT NULL DEFAULT '',
	ADD COLUMN ship_street      text NOT NULL DEFAULT '',
	ADD COLUMN ship_number      text NOT NULL DEFAULT '',
	ADD COLUMN ship_city        text NOT NULL DEFAULT '',
	ADD COLUMN ship_state       text NOT NULL DEFAULT '',
	ADD COLUMN ship_postal_code text NOT NULL DEFAULT '',
	ADD COLUMN shipment_id      text,
	ADD COLUMN tracking_code    text;

-- Charges at the gateway. At most one live charge per order.
CREATE TABLE payments (
	charge_id    text PRIMARY KEY,
	order_id     uuid NOT NULL REFERENCES orders (id),
	amount_cents bigint NOT NULL CHECK (amount_cents > 0),
	status       text NOT NULL CHECK (status IN ('pending', 'succeeded', 'failed')),
	created_at   timestamptz NOT NULL DEFAULT now(),
	updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX payments_one_live_per_order_idx ON payments (order_id) WHERE status IN ('pending', 'succeeded');

-- Every partner event seen: the primary key makes a replayed event a no-op.
CREATE TABLE partner_events (
	event_id    text PRIMARY KEY,
	partner     text NOT NULL CHECK (partner IN ('payments', 'shipping')),
	type        text NOT NULL,
	received_at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT ON payments TO orders;
GRANT UPDATE (status, updated_at) ON payments TO orders;
GRANT SELECT, INSERT ON partner_events TO orders;

GRANT SELECT ON payments TO panel_orders;
-- The panel creates the shipment at the carrier and keeps its references.
GRANT UPDATE (shipment_id, tracking_code) ON orders TO panel_orders;
