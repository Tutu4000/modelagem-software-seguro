-- Files whose last order was delivered before the cut, and files with no
-- live order created before the other cut. A live order is one that may still
-- need the design: anything but delivered, cancelled, rejected or an expired
-- quote.
-- name: PurgeableFiles :many
SELECT f.id
FROM files f
WHERE f.purged_at IS NULL
	AND NOT EXISTS (
		SELECT 1 FROM orders o
		WHERE o.file_id = f.id
			AND (o.status IN ('awaiting_review', 'awaiting_payment', 'paid', 'in_production', 'shipped')
				OR (o.status = 'quoted' AND o.quote_expires_at > now()))
	)
	AND (
		EXISTS (
			SELECT 1 FROM orders o JOIN order_status_history h ON h.order_id = o.id AND h.to_status = 'delivered'
			WHERE o.file_id = f.id AND h.at < @delivered_before
		)
		OR (
			f.created_at < @unordered_before
			AND NOT EXISTS (SELECT 1 FROM orders o WHERE o.file_id = f.id AND o.status = 'delivered')
		)
	)
	AND NOT EXISTS (
		SELECT 1 FROM orders o JOIN order_status_history h ON h.order_id = o.id AND h.to_status = 'delivered'
		WHERE o.file_id = f.id AND h.at >= @delivered_before
	)
LIMIT 500;

-- name: MarkFilePurged :exec
UPDATE files SET purged_at = now() WHERE id = @id;

-- Orders delivered or cancelled before the cut that still keep the address.
-- name: AddressesToPurge :many
SELECT o.id
FROM orders o
WHERE o.address_purged_at IS NULL
	AND o.status IN ('delivered', 'cancelled')
	AND EXISTS (
		SELECT 1 FROM order_status_history h
		WHERE h.order_id = o.id AND h.to_status = o.status AND h.at < @before
	)
LIMIT 500;

-- name: PurgeAddress :exec
UPDATE orders
SET ship_recipient = '', ship_street = '', ship_number = '', ship_city = '', ship_state = '', ship_postal_code = '',
	address_purged_at = now()
WHERE id = @id;
