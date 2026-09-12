-- name: GetEntityID :one
SELECT entity_id FROM entity_identities
WHERE zone_id = sqlc.arg(zone_id)
  AND title = sqlc.arg(title)
  AND entity_type = sqlc.arg(entity_type);

-- name: GetRelationID :one
SELECT relation_id FROM relation_identities
WHERE zone_id = sqlc.arg(zone_id)
  AND source_entity_id = sqlc.arg(source_entity_id)
  AND target_entity_id = sqlc.arg(target_entity_id)
  AND relation_type = sqlc.arg(relation_type);

-- name: GetClaimID :one
SELECT claim_id FROM claim_identities
WHERE zone_id = sqlc.arg(zone_id)
  AND subject_kind = sqlc.arg(subject_kind)
  AND subject_id = sqlc.arg(subject_id)
  AND claim_type = sqlc.arg(claim_type);

-- name: CreateEntityIdentity :exec
INSERT INTO entity_identities (zone_id, entity_id, title, entity_type)
VALUES (sqlc.arg(zone_id), sqlc.arg(entity_id), sqlc.arg(title), sqlc.arg(entity_type));

-- name: CreateRelationIdentity :exec
INSERT INTO relation_identities
    (zone_id, relation_id, source_entity_id, target_entity_id, relation_type)
VALUES
    (sqlc.arg(zone_id), sqlc.arg(relation_id), sqlc.arg(source_entity_id),
     sqlc.arg(target_entity_id), sqlc.arg(relation_type));

-- name: CreateClaimIdentity :exec
INSERT INTO claim_identities
    (zone_id, claim_id, subject_kind, subject_id, claim_type)
VALUES
    (sqlc.arg(zone_id), sqlc.arg(claim_id), sqlc.arg(subject_kind),
     sqlc.arg(subject_id), sqlc.arg(claim_type));

-- name: GetEntityIdentity :one
SELECT title, entity_type FROM entity_identities
WHERE zone_id = sqlc.arg(zone_id) AND entity_id = sqlc.arg(entity_id);

-- name: GetRelationIdentity :one
SELECT source_entity_id, target_entity_id, relation_type FROM relation_identities
WHERE zone_id = sqlc.arg(zone_id) AND relation_id = sqlc.arg(relation_id);

-- name: GetClaimIdentity :one
SELECT subject_kind, subject_id, claim_type FROM claim_identities
WHERE zone_id = sqlc.arg(zone_id) AND claim_id = sqlc.arg(claim_id);

-- name: HasEntityIdentity :one
SELECT count(*) FROM entity_identities
WHERE zone_id = sqlc.arg(zone_id) AND entity_id = sqlc.arg(entity_id);

-- name: HasRelationIdentity :one
SELECT count(*) FROM relation_identities
WHERE zone_id = sqlc.arg(zone_id) AND relation_id = sqlc.arg(relation_id);
