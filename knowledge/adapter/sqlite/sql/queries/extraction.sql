-- name: GetKnowledgeExtractionProgress :one
SELECT last_text_unit_id FROM knowledge_extraction_progress
WHERE zone_id = sqlc.arg(zone_id) AND corpora_id = sqlc.arg(corpora_id);

-- name: StartKnowledgeExtractionProgress :exec
INSERT INTO knowledge_extraction_progress (zone_id, corpora_id, last_text_unit_id)
VALUES (sqlc.arg(zone_id), sqlc.arg(corpora_id), sqlc.arg(last_text_unit_id));

-- name: AdvanceKnowledgeExtractionProgress :execrows
UPDATE knowledge_extraction_progress
SET last_text_unit_id = sqlc.arg(last_text_unit_id)
WHERE zone_id = sqlc.arg(zone_id) AND corpora_id = sqlc.arg(corpora_id)
  AND last_text_unit_id = sqlc.arg(expected_text_unit_id);
