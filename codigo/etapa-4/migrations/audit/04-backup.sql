-- Backup (E3.5): a copy of the trail, with a role that only reads.
GRANT CONNECT ON DATABASE audit TO backup;
GRANT pg_read_all_data TO backup;
