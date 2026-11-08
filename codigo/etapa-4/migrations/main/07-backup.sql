-- Backup (E3.5): the dump runs on the read replica with a role that only
-- reads. pg_read_all_data covers every table, present and future.
GRANT CONNECT ON DATABASE nugget TO backup;
GRANT pg_read_all_data TO backup;
