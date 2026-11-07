-- name: InsertEvent :exec
INSERT INTO eventos (horario, evento, autor, operacao, alvo, resultado, motivo, origem)
VALUES (@horario, @evento, @autor, @operacao, @alvo, @resultado, @motivo, @origem);
