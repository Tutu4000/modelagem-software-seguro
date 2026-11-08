-- Orders module (E8.1, E6.3). The API uses the "orders" role and the panel
-- the "panel_orders" role (D02). UPDATE is granted per column: not even a bug
-- in a binary can change a price through SQL (CA03).

GRANT CONNECT ON DATABASE nugget TO orders, panel_orders;
GRANT USAGE ON SCHEMA public TO orders, panel_orders;

-- Versioned price table. A new version only applies after a second admin
-- approves it (D02); there is at most one active version.
CREATE TABLE price_tables (
	version     bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	status      text NOT NULL CHECK (status IN ('draft', 'active', 'retired')),
	rules       jsonb NOT NULL,
	created_by  uuid,
	created_at  timestamptz NOT NULL DEFAULT now(),
	approved_by uuid,
	approved_at timestamptz,
	CHECK (approved_by IS NULL OR approved_by IS DISTINCT FROM created_by),
	CHECK (status = 'draft' OR approved_at IS NOT NULL)
);
CREATE UNIQUE INDEX price_tables_one_active_idx ON price_tables (status) WHERE status = 'active';

CREATE TABLE orders (
	id                  uuid PRIMARY KEY,
	owner_id            uuid NOT NULL,
	file_id             uuid NOT NULL REFERENCES files (id),
	material            text NOT NULL,
	color               text NOT NULL,
	quality             text NOT NULL,
	finishing           text NOT NULL,
	quantity            integer NOT NULL CHECK (quantity BETWEEN 1 AND 1000),
	price_table_version bigint NOT NULL REFERENCES price_tables (version),
	unit_cents          bigint NOT NULL CHECK (unit_cents >= 0),
	discount_cents      bigint NOT NULL CHECK (discount_cents >= 0),
	total_cents         bigint NOT NULL CHECK (total_cents >= 0),
	volume_mm3          double precision NOT NULL CHECK (volume_mm3 >= 0),
	bbox_mm             double precision[] NOT NULL,
	dfm_flags           text[] NOT NULL DEFAULT '{}',
	status              text NOT NULL CHECK (status IN (
		'quoted', 'awaiting_review', 'awaiting_payment', 'rejected',
		'paid', 'in_production', 'shipped', 'delivered', 'cancelled')),
	assigned_technician uuid,
	quote_expires_at    timestamptz NOT NULL,
	created_at          timestamptz NOT NULL DEFAULT now(),
	updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX orders_owner_id_created_at_idx ON orders (owner_id, created_at DESC);
CREATE INDEX orders_status_idx ON orders (status, created_at);
CREATE INDEX orders_assigned_technician_idx ON orders (assigned_technician, status);

-- Every status change, for the order history (RF12).
CREATE TABLE order_status_history (
	id          uuid PRIMARY KEY DEFAULT uuidv7(),
	order_id    uuid NOT NULL REFERENCES orders (id),
	from_status text,
	to_status   text NOT NULL,
	actor       uuid NOT NULL,
	at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX order_status_history_order_id_idx ON order_status_history (order_id, at);

-- One review per order: the primary key is the order (E6.3).
CREATE TABLE reviews (
	order_id      uuid PRIMARY KEY REFERENCES orders (id),
	technician_id uuid NOT NULL,
	decision      text NOT NULL CHECK (decision IN ('approved', 'rejected')),
	report        text NOT NULL CHECK (length(report) BETWEEN 10 AND 5000),
	created_at    timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT ON price_tables, reviews, order_status_history TO orders;
GRANT SELECT, INSERT ON orders TO orders;
GRANT UPDATE (status, updated_at) ON orders TO orders;
GRANT INSERT ON order_status_history TO orders;

GRANT SELECT ON price_tables, orders, reviews, order_status_history TO panel_orders;
GRANT UPDATE (status, assigned_technician, updated_at) ON orders TO panel_orders;
GRANT INSERT ON reviews, order_status_history TO panel_orders;
GRANT INSERT ON price_tables TO panel_orders;
GRANT UPDATE (status, approved_by, approved_at) ON price_tables TO panel_orders;

-- Version 1, the initial table, comes with the system and has no proposer
-- nor approver.
INSERT INTO price_tables (status, rules, approved_at) VALUES ('active', '{
	"materials": {
		"pla":   {"name": "PLA",        "cents_per_cm3": 15, "colors": ["black", "white", "red", "gray"]},
		"petg":  {"name": "PETG",       "cents_per_cm3": 20, "colors": ["black", "clear", "blue"]},
		"nylon": {"name": "Nylon PA12", "cents_per_cm3": 45, "colors": ["gray", "black"]}
	},
	"qualities":   {"draft": 800, "standard": 1000, "fine": 1500},
	"finishings":  {"none": 0, "sanded": 500, "painted": 1500},
	"setup_cents": 1000,
	"discounts":   [{"min_quantity": 10, "percent_off": 5}, {"min_quantity": 50, "percent_off": 10}]
}', now());
