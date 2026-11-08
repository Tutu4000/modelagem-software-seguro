-- name: InsertFile :exec
INSERT INTO files (id, owner_id, size_bytes, sha256)
VALUES (@id, @owner_id, @size_bytes, @sha256);

-- There is no lookup by ID alone: the owner is part of the query (D01).
-- name: FindFileForOwner :one
SELECT owner_id, size_bytes, sha256, purged_at
FROM files
WHERE id = @id AND owner_id = @owner_id;
