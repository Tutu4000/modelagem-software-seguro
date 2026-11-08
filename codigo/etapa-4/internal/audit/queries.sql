-- name: InsertEvent :exec
INSERT INTO events (occurred_at, name, actor, action, target, result, reason, origin)
VALUES (@occurred_at, @name, @actor, @action, @target, @result, @reason, @origin);
