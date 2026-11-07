-- name: InsertFile :exec
INSERT INTO arquivos (id, dono, tamanho, sha256)
VALUES (@id, @dono, @tamanho, @sha256);

-- Não existe busca só por ID: o dono faz parte da consulta (D01).
-- name: FindFileForOwner :one
SELECT dono, tamanho, sha256
FROM arquivos
WHERE id = @id AND dono = @dono;
