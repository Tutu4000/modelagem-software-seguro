-- Serializes the quota decisions of one account across all API replicas,
-- until the end of the transaction.
-- name: LockOwner :exec
SELECT pg_advisory_xact_lock(hashtextextended(@owner_id::text, 0));

-- name: CountProcessing :one
SELECT count(*)
FROM tasks
WHERE owner_id = @owner_id AND status = 'processing';

-- name: InsertTask :exec
INSERT INTO tasks (id, file_id, owner_id, sha256, triangles, status)
VALUES (@id, @file_id, @owner_id, @sha256, @triangles, @status);

-- name: TaskStatusByFile :one
SELECT status, volume_mm3, bbox_mm
FROM tasks
WHERE file_id = @file_id;

-- name: TaskOwner :one
SELECT owner_id FROM tasks WHERE id = @id;

-- Only a task that is processing changes state: a repeated result affects no
-- rows (E4.6).
-- name: FinishTask :execrows
UPDATE tasks
SET status = @status, volume_mm3 = @volume_mm3, bbox_mm = @bbox_mm, updated_at = now()
WHERE id = @id AND status = 'processing';

-- name: OldestWaiting :one
SELECT id, file_id, sha256
FROM tasks
WHERE owner_id = @owner_id AND status = 'waiting'
ORDER BY created_at
LIMIT 1;

-- name: StartTask :exec
UPDATE tasks
SET status = 'processing', updated_at = now()
WHERE id = @id;
