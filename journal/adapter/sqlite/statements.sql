-- name: GetSchemaVersion :one
SELECT version
FROM journal_schema
WHERE id = 1;

-- name: ValidateSchema :one
SELECT
    (SELECT
        count(event_id) +
        count(zone_id) +
        count(stream_id) +
        count(stream_sequence) +
        count(event_type) +
        count(event_sequence) +
        count(schema_version) +
        count(occurred_at) +
        count(correlation_id) +
        count(causation_id) +
        count(body)
     FROM journal_events INDEXED BY journal_events_by_stream
     WHERE 0) +
    (SELECT count(event_sequence)
     FROM journal_events INDEXED BY journal_events_by_zone_key_sequence
     WHERE 0) +
    (SELECT count(zone_id) + count(consumer_id) + count(event_type) + count(position)
     FROM journal_consumer_positions
     WHERE 0) +
    (SELECT count(id) + count(version)
     FROM journal_schema
     WHERE 0) AS valid;

-- These named statements are the sole generated CRUD source for Journal persistence.
-- name: GetEventByID :one
SELECT *
FROM journal_events
WHERE event_id = ?;

-- name: GetLastStreamSequence :one
SELECT CAST(COALESCE(MAX(stream_sequence), 0) AS INTEGER)
FROM journal_events
WHERE zone_id = ? AND stream_id = ?;

-- name: GetEventTypePosition :one
SELECT CAST(COALESCE(MAX(event_sequence) + 1, 0) AS INTEGER)
FROM journal_events
WHERE zone_id = sqlc.arg(zone_id) AND event_type = sqlc.arg(event_type);

-- name: CreateEvent :exec
INSERT INTO journal_events (
    event_id,
    zone_id,
    stream_id,
    stream_sequence,
    event_type,
    event_sequence,
    schema_version,
    occurred_at,
    correlation_id,
    causation_id,
    body
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListEventTypeEvents :many
SELECT *
FROM journal_events
WHERE zone_id = sqlc.arg(zone_id)
  AND event_type = sqlc.arg(event_type)
  AND event_sequence >= sqlc.arg(position)
ORDER BY event_sequence
LIMIT sqlc.arg(read_limit);

-- name: ListJournalEntries :many
SELECT *
FROM journal_events
WHERE zone_id = sqlc.arg(zone_id)
ORDER BY event_type, event_sequence
LIMIT sqlc.arg(read_limit) OFFSET sqlc.arg(entry_offset);

-- name: ReadPendingEvents :one
SELECT
    CAST(COALESCE(MAX(event_sequence) + 1, 0) AS INTEGER) AS event_type_position,
    CAST(COUNT(CASE
        WHEN schema_version = sqlc.arg(schema_version)
         AND event_sequence >= sqlc.arg(event_sequence)
        THEN 1
    END) AS INTEGER) AS event_count,
    MIN(CASE
        WHEN schema_version = sqlc.arg(schema_version)
         AND event_sequence >= sqlc.arg(event_sequence)
        THEN occurred_at
    END) AS pending_since
FROM journal_events INDEXED BY journal_events_by_zone_key_sequence
WHERE zone_id = sqlc.arg(zone_id)
  AND event_type = sqlc.arg(event_type);

-- name: ListStreamEventsAfter :many
SELECT *
FROM journal_events
WHERE zone_id = ? AND stream_id = ? AND stream_sequence > ?
ORDER BY stream_sequence
LIMIT ?;

-- name: GetConsumerPosition :one
SELECT position
FROM journal_consumer_positions
WHERE zone_id = sqlc.arg(zone_id)
  AND consumer_id = sqlc.arg(consumer_id)
  AND event_type = sqlc.arg(event_type);

-- name: EnsureConsumerPosition :exec
INSERT INTO journal_consumer_positions (zone_id, consumer_id, event_type, position)
VALUES (
    sqlc.arg(zone_id),
    sqlc.arg(consumer_id),
    sqlc.arg(event_type),
    0
)
ON CONFLICT (zone_id, consumer_id, event_type) DO NOTHING;

-- name: AdvanceConsumerPosition :execrows
UPDATE journal_consumer_positions
SET position = sqlc.arg(position)
WHERE zone_id = sqlc.arg(zone_id)
  AND consumer_id = sqlc.arg(consumer_id)
  AND event_type = sqlc.arg(event_type)
  AND position = sqlc.arg(expected_position);

-- name: GetPendingConsumerZone :one
SELECT events.zone_id
FROM journal_events AS events
LEFT JOIN journal_consumer_positions AS positions
  ON positions.zone_id = events.zone_id
 AND positions.consumer_id = sqlc.arg(consumer_id)
 AND positions.event_type = events.event_type
WHERE events.event_type = sqlc.arg(event_type)
  AND events.schema_version = sqlc.arg(schema_version)
  AND events.event_sequence >= COALESCE(positions.position, 0)
GROUP BY events.zone_id
ORDER BY MIN(events.occurred_at), events.zone_id
LIMIT 1;
