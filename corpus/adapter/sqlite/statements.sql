-- name: GetSchemaVersion :one
SELECT version FROM corpus_schema WHERE id = 1;

-- name: ValidateSchema :one
SELECT
    (SELECT count(*) FROM corpus_child_boundaries WHERE 0) +
    (SELECT count(*) FROM corpus_text_vector_progress WHERE 0) +
    (SELECT count(*) FROM documents WHERE 0) +
    (SELECT count(*) FROM zone_documents WHERE 0) +
    (SELECT count(*) FROM texts WHERE 0) +
    (SELECT count(*) FROM corpus_local_texts WHERE 0) +
    (SELECT count(*) FROM text_warnings WHERE 0) +
    (SELECT count(*) FROM text_units WHERE 0) +
    (SELECT count(*) FROM messages WHERE 0) +
    (SELECT count(*) FROM text_unit_spans WHERE 0) +
    (SELECT count(*) FROM text_chunking_progress WHERE 0) +
    (SELECT count(*) FROM corpus_state WHERE 0) +
    (SELECT count(*) FROM texts INDEXED BY texts_by_document_and_profiles WHERE 0) +
    (SELECT count(*) FROM text_unit_spans INDEXED BY text_unit_spans_by_set_and_text_unit WHERE 0) AS valid;

-- name: SaveDocument :execrows
INSERT OR IGNORE INTO documents (id, name, media_type, size, digest, location)
VALUES (?, ?, ?, ?, ?, ?);

-- name: SaveZoneDocument :execrows
INSERT OR IGNORE INTO zone_documents (zone_id, document_id)
VALUES (?, ?);

-- name: GetDocument :one
SELECT document_value.id, document_value.name, document_value.media_type,
       document_value.size, document_value.digest, document_value.location
FROM documents AS document_value
JOIN zone_documents AS zone_document
  ON zone_document.document_id = document_value.id
WHERE zone_document.zone_id = ? AND document_value.id = ?;

-- name: GetStoredDocument :one
SELECT id, name, media_type, size, digest, location
FROM documents WHERE id = ?;

-- name: GetDocumentByDigest :one
SELECT id, name, media_type, size, digest, location
FROM documents WHERE digest = ?;

-- name: GetDocumentByLocation :one
SELECT document_value.id, document_value.name, document_value.media_type,
       document_value.size, document_value.digest, document_value.location
FROM documents AS document_value
JOIN zone_documents AS zone_document
  ON zone_document.document_id = document_value.id
WHERE zone_document.zone_id = ? AND document_value.location = ?;

-- name: ListDocuments :many
SELECT document_value.id, document_value.name, document_value.media_type,
       document_value.size, document_value.digest, document_value.location
FROM documents AS document_value
JOIN zone_documents AS zone_document
  ON zone_document.document_id = document_value.id
WHERE zone_document.zone_id = ?
ORDER BY document_value.id LIMIT ? OFFSET ?;

-- name: BrowseDocuments :many
SELECT document_value.id, document_value.name, document_value.media_type,
       document_value.size, document_value.digest,
       CASE WHEN zone_document.document_id IS NULL THEN 0 ELSE 1 END AS selected
FROM documents AS document_value
LEFT JOIN zone_documents AS zone_document
  ON zone_document.zone_id = ? AND zone_document.document_id = document_value.id
ORDER BY document_value.id LIMIT ? OFFSET ?;

-- name: SaveText :execrows
INSERT OR IGNORE INTO texts (
    id, source_document_id, title, body, format,
    extraction_profile, normalization_profile
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: MarkLocalText :exec
INSERT OR IGNORE INTO corpus_local_texts (zone_id, text_id) VALUES (?, ?);

-- name: GetText :one
SELECT id, source_document_id, title, body, format,
       extraction_profile, normalization_profile
FROM texts AS text_value
JOIN corpus_local_texts AS local_text ON local_text.text_id = text_value.id
WHERE local_text.zone_id = ? AND text_value.id = ?;

-- name: GetStoredText :one
SELECT id, source_document_id, title, body, format,
       extraction_profile, normalization_profile
FROM texts WHERE id = ?;

-- name: FindText :one
SELECT id, source_document_id, title, body, format,
       extraction_profile, normalization_profile
FROM texts AS text_value
JOIN corpus_local_texts AS local_text ON local_text.text_id = text_value.id
WHERE local_text.zone_id = ?
  AND text_value.source_document_id = ?
  AND extraction_profile = ?
  AND normalization_profile = ?;

-- name: FindStoredText :one
SELECT id, source_document_id, title, body, format,
       extraction_profile, normalization_profile
FROM texts
WHERE source_document_id = ?
  AND extraction_profile = ?
  AND normalization_profile = ?;

-- name: ListTexts :many
SELECT id, source_document_id, title, body, format,
       extraction_profile, normalization_profile
FROM texts AS text_value
JOIN corpus_local_texts AS local_text ON local_text.text_id = text_value.id
WHERE local_text.zone_id = ? ORDER BY text_value.id LIMIT ? OFFSET ?;

-- name: CountTexts :one
SELECT count(*)
FROM corpus_local_texts
WHERE zone_id = ?;

-- name: AddTextWarning :exec
INSERT INTO text_warnings (text_id, warning_position, warning)
VALUES (?, ?, ?);

-- name: ListTextWarnings :many
SELECT text_id, warning_position, warning
FROM text_warnings WHERE text_id = ? ORDER BY warning_position;

-- name: EnsureCorpusState :exec
INSERT OR IGNORE INTO corpus_state (
    zone_id, current_corpora_id, building_corpora_id, building_state
) VALUES (?, 0, NULL, NULL);

-- name: GetCorpusState :one
SELECT current_corpora_id, building_corpora_id, building_state
FROM corpus_state WHERE zone_id = ?;

-- name: StartCorpusBuild :execrows
UPDATE corpus_state
SET building_corpora_id = current_corpora_id + 1, building_state = 'building'
WHERE zone_id = ? AND building_corpora_id IS NULL AND building_state IS NULL;

-- name: MarkCorpusPrepared :execrows
UPDATE corpus_state
SET building_state = 'prepared'
WHERE zone_id = sqlc.arg(zone_id)
  AND building_corpora_id = sqlc.arg(building_corpora_id)
  AND building_state = 'building';

-- name: ActivateCorpus :execrows
UPDATE corpus_state
SET current_corpora_id = building_corpora_id,
    building_corpora_id = NULL,
    building_state = NULL
WHERE zone_id = sqlc.arg(zone_id)
  AND building_corpora_id = sqlc.arg(building_corpora_id)
  AND building_state = 'prepared';

-- name: CopyTextUnitSpans :exec
INSERT INTO text_unit_spans (
    zone_id, corpora_id, text_id, text_unit_id, start_char, end_char, token_count
)
SELECT sqlc.arg(zone_id), sqlc.arg(target_corpora_id), source.text_id,
       source.text_unit_id, source.start_char, source.end_char, source.token_count
FROM text_unit_spans AS source
WHERE source.zone_id = sqlc.arg(zone_id)
  AND source.corpora_id = sqlc.arg(source_corpora_id)
  AND EXISTS (
      SELECT 1 FROM corpus_local_texts AS local_text
      WHERE local_text.zone_id = source.zone_id
        AND local_text.text_id = source.text_id
  );

-- name: DeleteCorporaTextUnitSpans :exec
DELETE FROM text_unit_spans
WHERE zone_id = ? AND corpora_id = ? AND text_id = ?;

-- name: CreateTextChunkingProgress :exec
INSERT INTO text_chunking_progress (
    zone_id, text_id, source_event_id, next_chunk_index, completed
) VALUES (?, ?, ?, 0, 0);

-- name: GetTextChunkingProgressBySource :one
SELECT text_id, source_event_id, next_chunk_index, completed
FROM text_chunking_progress
WHERE zone_id = ? AND source_event_id = ?;

-- name: GetTextChunkingProgress :one
SELECT text_id, source_event_id, next_chunk_index, completed
FROM text_chunking_progress
WHERE zone_id = ? AND text_id = ?;

-- name: AdvanceTextChunkingProgress :execrows
UPDATE text_chunking_progress
SET next_chunk_index = sqlc.arg(next_chunk_index), completed = sqlc.arg(completed)
WHERE zone_id = sqlc.arg(zone_id)
  AND text_id = sqlc.arg(text_id)
  AND next_chunk_index = sqlc.arg(expected_next_chunk_index)
  AND completed = 0;

-- name: CountIncompleteTextChunking :one
SELECT count(*) FROM text_chunking_progress WHERE zone_id = ? AND completed = 0;

-- name: ClearTextChunkingProgress :exec
DELETE FROM text_chunking_progress WHERE zone_id = ?;

-- name: CountCorporaTexts :one
SELECT count(DISTINCT text_id)
FROM text_unit_spans WHERE zone_id = ? AND corpora_id = ?;

-- name: SaveTextUnit :execrows
INSERT OR IGNORE INTO text_units (zone_id, id, text) VALUES (?, ?, ?);

-- name: GetTextUnit :one
SELECT id, text FROM text_units WHERE zone_id = ? AND id = ?;

-- name: LastMessagePosition :one
SELECT CAST(coalesce(max(position), -1) AS INTEGER) AS position
FROM messages WHERE zone_id = ?;

-- name: SaveMessage :execrows
INSERT INTO messages (zone_id, id, role, position, text_unit_id)
VALUES (?, ?, ?, ?, ?);

-- name: GetMessage :one
SELECT message.id, message.role, message.position,
       message.text_unit_id, unit.text AS text_unit_text
FROM messages AS message
JOIN text_units AS unit
  ON unit.zone_id = message.zone_id AND unit.id = message.text_unit_id
WHERE message.zone_id = ? AND message.id = ?;

-- name: ListMessages :many
SELECT message.id, message.role, message.position,
       message.text_unit_id, unit.text AS text_unit_text
FROM messages AS message
JOIN text_units AS unit
  ON unit.zone_id = message.zone_id AND unit.id = message.text_unit_id
WHERE message.zone_id = sqlc.arg(zone_id)
  AND message.id IN (sqlc.slice('message_ids'))
ORDER BY message.position;

-- name: ListTextUnitsBeforeCorpora :many
SELECT DISTINCT unit.id, unit.text
FROM text_units AS unit
JOIN text_unit_spans AS span
  ON span.zone_id = unit.zone_id AND span.text_unit_id = unit.id
WHERE unit.zone_id = sqlc.arg(zone_id)
  AND span.corpora_id < sqlc.arg(exclusive_corpora_id)
  AND unit.id IN (sqlc.slice('text_unit_ids'))
ORDER BY unit.id;

-- name: AddTextUnitSpan :exec
INSERT INTO text_unit_spans (
    zone_id, corpora_id, text_id, text_unit_id, start_char, end_char, token_count
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: ListTextUnitSpans :many
SELECT span.corpora_id, span.text_id, span.text_unit_id,
       span.start_char, span.end_char, span.token_count,
       unit.text AS text_unit_text
FROM text_unit_spans AS span
JOIN text_units AS unit
  ON unit.zone_id = span.zone_id AND unit.id = span.text_unit_id
WHERE span.zone_id = ? AND span.corpora_id = ?
ORDER BY span.text_id, span.start_char, span.end_char;

-- name: ListCorporaTextUnitSpans :many
SELECT span.text_unit_id, span.start_char, span.end_char, span.token_count,
       unit.text AS text_unit_text
FROM text_unit_spans AS span
JOIN text_units AS unit
  ON unit.zone_id = span.zone_id AND unit.id = span.text_unit_id
WHERE span.zone_id = sqlc.arg(zone_id)
  AND span.corpora_id = sqlc.arg(corpora_id)
  AND span.text_id = sqlc.arg(text_id)
ORDER BY span.start_char, span.end_char;

-- name: CountTextUnitSpans :one
SELECT count(*) FROM text_unit_spans WHERE zone_id = ? AND corpora_id = ?;

-- name: ListTextUnitLocations :many
SELECT entry.corpora_id,
       entry.text_id,
       entry.start_char,
       entry.end_char,
       entry.token_count,
       text_value.id AS text_id,
       text_value.title,
       text_value.body AS text_body,
       text_value.source_document_id,
       document_value.location AS document_location,
       unit.text AS text_unit_text,
       unit.id AS text_unit_id
FROM text_unit_spans AS entry
JOIN texts AS text_value
  ON text_value.id = entry.text_id
JOIN documents AS document_value
  ON document_value.id = text_value.source_document_id
JOIN zone_documents AS zone_document
  ON zone_document.zone_id = entry.zone_id
 AND zone_document.document_id = document_value.id
JOIN text_units AS unit
  ON unit.zone_id = entry.zone_id AND unit.id = entry.text_unit_id
WHERE entry.zone_id = sqlc.arg(zone_id)
  AND entry.corpora_id = sqlc.arg(corpora_id)
  AND entry.text_unit_id IN (sqlc.slice('text_unit_ids'))
ORDER BY entry.text_id, entry.start_char, entry.end_char;

-- name: GetCurrentCorporaID :one
SELECT current_corpora_id FROM corpus_state WHERE zone_id = ?;

-- name: GetChildCorporaBoundary :one
SELECT source_corpora_id
FROM corpus_child_boundaries
WHERE zone_id = sqlc.arg(zone_id) AND child_zone_id = sqlc.arg(child_zone_id);

-- name: SaveChildCorporaBoundary :exec
INSERT INTO corpus_child_boundaries (zone_id, child_zone_id, source_corpora_id)
VALUES (sqlc.arg(zone_id), sqlc.arg(child_zone_id), sqlc.arg(source_corpora_id))
ON CONFLICT (zone_id, child_zone_id) DO UPDATE
SET source_corpora_id = excluded.source_corpora_id;

-- name: ListChildCorporaBoundaries :many
SELECT child_zone_id, source_corpora_id
FROM corpus_child_boundaries
WHERE zone_id = sqlc.arg(zone_id)
ORDER BY child_zone_id;
