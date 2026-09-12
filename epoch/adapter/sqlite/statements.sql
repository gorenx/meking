-- name: GetSchemaVersion :one
SELECT version FROM epoch_schema WHERE id = 1;

-- name: ValidateSchema :one
SELECT
	(SELECT count(zone_id) + count(id) + count(knowledge_digest) + count(corpora_id) + count(structure_id) + count(published_at) FROM epochs WHERE 0) +
	(SELECT count(*) FROM epoch_entity_versions WHERE 0) +
	(SELECT count(*) FROM epoch_relation_versions WHERE 0) +
	(SELECT count(*) FROM epoch_claim_versions WHERE 0) +
	(SELECT count(zone_id) + count(epoch_id) FROM current_epoch WHERE 0) +
	(SELECT count(*) FROM epochs INDEXED BY epochs_by_structure WHERE 0) +
    (SELECT count(id) + count(version) FROM epoch_schema WHERE 0) AS valid;

-- name: CreateEpoch :one
INSERT INTO epochs (zone_id, id, knowledge_digest, corpora_id, structure_id, published_at)
VALUES (
    sqlc.arg(zone_id),
    (SELECT COALESCE(MAX(id), 0) + 1 FROM epochs WHERE zone_id = sqlc.arg(zone_id)),
    sqlc.arg(knowledge_digest), sqlc.arg(corpora_id), sqlc.arg(structure_id), sqlc.arg(published_at)
)
RETURNING id;

-- name: AddEpochEntityVersion :exec
INSERT INTO epoch_entity_versions (zone_id, epoch_id, entity_id, version)
VALUES (sqlc.arg(zone_id), sqlc.arg(epoch_id), sqlc.arg(entity_id), sqlc.arg(version));

-- name: AddEpochRelationVersion :exec
INSERT INTO epoch_relation_versions (zone_id, epoch_id, relation_id, version)
VALUES (sqlc.arg(zone_id), sqlc.arg(epoch_id), sqlc.arg(relation_id), sqlc.arg(version));

-- name: AddEpochClaimVersion :exec
INSERT INTO epoch_claim_versions (zone_id, epoch_id, claim_id, version)
VALUES (sqlc.arg(zone_id), sqlc.arg(epoch_id), sqlc.arg(claim_id), sqlc.arg(version));

-- name: ListEpochEntityVersions :many
SELECT entity_id, version FROM epoch_entity_versions
WHERE zone_id = sqlc.arg(zone_id) AND epoch_id = sqlc.arg(epoch_id)
ORDER BY entity_id;

-- name: ListEpochRelationVersions :many
SELECT relation_id, version FROM epoch_relation_versions
WHERE zone_id = sqlc.arg(zone_id) AND epoch_id = sqlc.arg(epoch_id)
ORDER BY relation_id;

-- name: ListEpochClaimVersions :many
SELECT claim_id, version FROM epoch_claim_versions
WHERE zone_id = sqlc.arg(zone_id) AND epoch_id = sqlc.arg(epoch_id)
ORDER BY claim_id;

-- name: GetEpoch :one
SELECT * FROM epochs WHERE zone_id = ? AND id = ?;

-- name: GetEpochByStructure :one
SELECT * FROM epochs
WHERE zone_id = sqlc.arg(zone_id) AND structure_id = sqlc.arg(structure_id);

-- name: CountEpochs :one
SELECT count(*) FROM epochs WHERE zone_id = ?;

-- name: GetCurrentEpochID :one
SELECT epoch_id FROM current_epoch WHERE zone_id = ?;

-- name: SelectCurrentEpoch :exec
INSERT INTO current_epoch (zone_id, epoch_id) VALUES (?, ?)
ON CONFLICT(zone_id) DO UPDATE SET epoch_id = excluded.epoch_id;
