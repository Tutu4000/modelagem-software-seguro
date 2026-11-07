-- Each module reaches only its own tables (E3.1, C02). The tables belong to
-- the superuser, which the application never uses.

REVOKE ALL ON DATABASE nugget FROM PUBLIC;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT CONNECT ON DATABASE nugget TO files, queue;
GRANT USAGE ON SCHEMA public TO files, queue;

-- Files module
CREATE TABLE files (
	id         uuid PRIMARY KEY,
	owner_id   uuid NOT NULL,
	size_bytes bigint NOT NULL CHECK (size_bytes > 0),
	sha256     bytea NOT NULL CHECK (length(sha256) = 32),
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX files_owner_id_idx ON files (owner_id);
GRANT SELECT, INSERT ON files TO files;

-- Queue module: task state and the per-account quota (E4.3)
CREATE TABLE tasks (
	id         uuid PRIMARY KEY,
	file_id    uuid NOT NULL UNIQUE REFERENCES files (id),
	owner_id   uuid NOT NULL,
	sha256     bytea NOT NULL CHECK (length(sha256) = 32),
	triangles  bigint NOT NULL CHECK (triangles >= 0),
	status     text NOT NULL CHECK (status IN ('waiting', 'processing', 'done', 'failed')),
	volume_mm3 double precision,
	bbox_mm    double precision[],
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tasks_owner_id_status_idx ON tasks (owner_id, status, created_at);
GRANT SELECT, INSERT, UPDATE ON tasks TO queue;
