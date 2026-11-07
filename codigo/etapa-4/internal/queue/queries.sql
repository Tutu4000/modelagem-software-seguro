-- Serializa as decisões de cota de uma conta entre as réplicas da API, até o
-- fim da transação.
-- name: LockOwner :exec
SELECT pg_advisory_xact_lock(hashtextextended(@dono::text, 0));

-- name: CountProcessing :one
SELECT count(*)
FROM tarefas
WHERE dono = @dono AND status = 'processando';

-- name: InsertTask :exec
INSERT INTO tarefas (id, arquivo_id, dono, sha256, triangulos, status)
VALUES (@id, @arquivo_id, @dono, @sha256, @triangulos, @status);

-- name: TaskStatusByFile :one
SELECT status, volume_mm3, caixa_mm
FROM tarefas
WHERE arquivo_id = @arquivo_id;

-- name: TaskOwner :one
SELECT dono FROM tarefas WHERE id = @id;

-- Só uma tarefa em processamento muda de estado: um resultado repetido não
-- afeta nenhuma linha (E4.6).
-- name: FinishTask :execrows
UPDATE tarefas
SET status = @status, volume_mm3 = @volume_mm3, caixa_mm = @caixa_mm, atualizado_em = now()
WHERE id = @id AND status = 'processando';

-- name: OldestWaiting :one
SELECT id, arquivo_id, sha256
FROM tarefas
WHERE dono = @dono AND status = 'aguardando'
ORDER BY criado_em
LIMIT 1;

-- name: StartTask :exec
UPDATE tarefas
SET status = 'processando', atualizado_em = now()
WHERE id = @id;
