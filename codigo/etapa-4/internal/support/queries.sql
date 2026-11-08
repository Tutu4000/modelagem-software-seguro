-- name: InsertTicket :exec
INSERT INTO tickets (id, owner_id, order_id, subject, status, contact_email)
VALUES (@id, @owner_id, @order_id, @subject, 'open', @contact_email);

-- name: InsertMessage :exec
INSERT INTO ticket_messages (ticket_id, author_id, author_kind, body)
VALUES (@ticket_id, @author_id, @author_kind, @body);

-- A ticket can only point at an order of the same customer (D01).
-- name: OrderOwnedBy :one
SELECT EXISTS (SELECT 1 FROM orders WHERE id = @order_id AND owner_id = @owner_id) AS owned;

-- name: CountOpenTickets :one
SELECT count(*) FROM tickets WHERE owner_id = @owner_id AND status <> 'closed';

-- There is no customer lookup by ID alone: the owner is part of the query.
-- name: GetTicketForOwner :one
SELECT * FROM tickets WHERE id = @id AND owner_id = @owner_id;

-- For the panel, which checks the role.
-- name: GetTicket :one
SELECT * FROM tickets WHERE id = @id;

-- name: ListTicketsForOwner :many
SELECT * FROM tickets WHERE owner_id = @owner_id ORDER BY created_at DESC LIMIT 100;

-- name: ListTicketsByStatus :many
SELECT * FROM tickets
WHERE (sqlc.arg(status)::text = '' OR status = sqlc.arg(status)::text)
ORDER BY updated_at DESC
LIMIT 200;

-- name: ListMessages :many
SELECT id, author_kind, body, created_at FROM ticket_messages WHERE ticket_id = @ticket_id ORDER BY created_at;

-- A closed ticket stays closed.
-- name: SetTicketStatus :execrows
UPDATE tickets SET status = @status, updated_at = now() WHERE id = @id AND status <> 'closed';
