-- Monitoring (E7.4, C19). Grafana reads the business numbers through a role
-- that sees no personal data: no address, e-mail, owner, ticket text or code
-- hash. The exporter only reads the server statistics.

GRANT CONNECT ON DATABASE nugget TO grafana_main, exporter;
GRANT USAGE ON SCHEMA public TO grafana_main;

GRANT SELECT (id, status, material, quantity, discount_cents, shipping_cents, total_cents, created_at, updated_at)
	ON orders TO grafana_main;
GRANT SELECT (order_id, from_status, to_status, at) ON order_status_history TO grafana_main;
GRANT SELECT (order_id, amount_cents, status, created_at, updated_at) ON payments TO grafana_main;
GRANT SELECT (id, status, created_at, updated_at) ON tickets TO grafana_main;
GRANT SELECT (id, size_bytes, created_at, purged_at) ON files TO grafana_main;

GRANT pg_monitor TO exporter;
