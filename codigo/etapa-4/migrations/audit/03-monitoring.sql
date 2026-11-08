-- Monitoring (E7.4, C19). Grafana reads the trail and cannot change it; the
-- exporter only reads the server statistics.

GRANT CONNECT ON DATABASE audit TO grafana_reader, exporter;
GRANT USAGE ON SCHEMA public TO grafana_reader;
GRANT SELECT ON events TO grafana_reader;

GRANT pg_monitor TO exporter;
