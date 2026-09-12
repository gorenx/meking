-- name: GetCurrentEntity :one
SELECT i.title, i.entity_type, v.version, v.deleted, v.content_hash, v.description
FROM entity_identities i
JOIN knowledge_versions v
  ON v.zone_id = i.zone_id AND v.knowledge_kind = 'entity'
 AND v.knowledge_id = i.entity_id
WHERE i.zone_id = sqlc.arg(zone_id) AND i.entity_id = sqlc.arg(entity_id)
  AND v.candidate_number = 0
ORDER BY v.version DESC
LIMIT 1;

-- name: GetEntityVersion :one
SELECT i.title, i.entity_type, v.version, v.deleted, v.content_hash, v.description
FROM entity_identities i
JOIN knowledge_versions v
  ON v.zone_id = i.zone_id AND v.knowledge_kind = 'entity'
 AND v.knowledge_id = i.entity_id
WHERE i.zone_id = sqlc.arg(zone_id) AND i.entity_id = sqlc.arg(entity_id)
  AND v.version = sqlc.arg(version) AND v.candidate_number = 0;

-- name: FindEntities :many
SELECT i.entity_id, v.version
FROM entity_identities i
JOIN knowledge_versions v
  ON v.zone_id = i.zone_id AND v.knowledge_kind = 'entity'
 AND v.knowledge_id = i.entity_id
WHERE i.zone_id = sqlc.arg(zone_id)
  AND i.title = sqlc.arg(title)
  AND (CAST(sqlc.arg(entity_type) AS TEXT) = '' OR i.entity_type = CAST(sqlc.arg(entity_type) AS TEXT))
  AND v.candidate_number = 0
  AND v.deleted = 0
  AND v.version = (
      SELECT MAX(current.version) FROM knowledge_versions current
      WHERE current.zone_id = v.zone_id
        AND current.knowledge_kind = v.knowledge_kind
        AND current.knowledge_id = v.knowledge_id
        AND current.candidate_number = 0
  )
ORDER BY i.entity_type, i.entity_id
LIMIT sqlc.arg(page_size);

-- name: ListEntityAliases :many
SELECT alias FROM entity_version_aliases
WHERE zone_id = sqlc.arg(zone_id)
  AND entity_id = sqlc.arg(entity_id)
  AND version = sqlc.arg(version) AND candidate_number = 0
ORDER BY alias;

-- name: GetCurrentRelation :one
SELECT i.source_entity_id, i.target_entity_id, i.relation_type,
       v.version, v.deleted, v.content_hash, v.description
FROM relation_identities i
JOIN knowledge_versions v
  ON v.zone_id = i.zone_id AND v.knowledge_kind = 'relation'
 AND v.knowledge_id = i.relation_id
WHERE i.zone_id = sqlc.arg(zone_id) AND i.relation_id = sqlc.arg(relation_id)
  AND v.candidate_number = 0
ORDER BY v.version DESC
LIMIT 1;

-- name: GetRelationVersion :one
SELECT i.source_entity_id, i.target_entity_id, i.relation_type,
       v.version, v.deleted, v.content_hash, v.description
FROM relation_identities i
JOIN knowledge_versions v
  ON v.zone_id = i.zone_id AND v.knowledge_kind = 'relation'
 AND v.knowledge_id = i.relation_id
WHERE i.zone_id = sqlc.arg(zone_id) AND i.relation_id = sqlc.arg(relation_id)
  AND v.version = sqlc.arg(version) AND v.candidate_number = 0;

-- name: GetCurrentClaim :one
SELECT i.subject_kind, i.subject_id, i.claim_type,
       v.version, v.deleted, v.content_hash, v.description
FROM claim_identities i
JOIN knowledge_versions v
  ON v.zone_id = i.zone_id AND v.knowledge_kind = 'claim'
 AND v.knowledge_id = i.claim_id
WHERE i.zone_id = sqlc.arg(zone_id) AND i.claim_id = sqlc.arg(claim_id)
  AND v.candidate_number = 0
ORDER BY v.version DESC
LIMIT 1;

-- name: GetClaimVersion :one
SELECT i.subject_kind, i.subject_id, i.claim_type,
       v.version, v.deleted, v.content_hash, v.description
FROM claim_identities i
JOIN knowledge_versions v
  ON v.zone_id = i.zone_id AND v.knowledge_kind = 'claim'
 AND v.knowledge_id = i.claim_id
WHERE i.zone_id = sqlc.arg(zone_id) AND i.claim_id = sqlc.arg(claim_id)
  AND v.version = sqlc.arg(version) AND v.candidate_number = 0;

-- name: ListActiveEntityReferences :many
SELECT v.knowledge_id AS entity_id, v.version
FROM knowledge_versions v
WHERE v.zone_id = sqlc.arg(zone_id) AND v.knowledge_kind = 'entity'
  AND v.candidate_number = 0 AND v.deleted = 0
  AND v.version = (
      SELECT MAX(current.version) FROM knowledge_versions current
      WHERE current.zone_id = v.zone_id
        AND current.knowledge_kind = v.knowledge_kind
        AND current.knowledge_id = v.knowledge_id
        AND current.candidate_number = 0
  )
  AND v.knowledge_id > sqlc.arg(after_entity_id)
ORDER BY v.knowledge_id
LIMIT sqlc.arg(page_size);

-- name: ListActiveRelationReferences :many
SELECT v.knowledge_id AS relation_id, v.version
FROM knowledge_versions v
WHERE v.zone_id = sqlc.arg(zone_id) AND v.knowledge_kind = 'relation'
  AND v.candidate_number = 0 AND v.deleted = 0
  AND v.version = (
      SELECT MAX(current.version) FROM knowledge_versions current
      WHERE current.zone_id = v.zone_id
        AND current.knowledge_kind = v.knowledge_kind
        AND current.knowledge_id = v.knowledge_id
        AND current.candidate_number = 0
  )
  AND v.knowledge_id > sqlc.arg(after_relation_id)
ORDER BY v.knowledge_id
LIMIT sqlc.arg(page_size);

-- name: ListActiveClaimReferences :many
SELECT v.knowledge_id AS claim_id, v.version
FROM knowledge_versions v
WHERE v.zone_id = sqlc.arg(zone_id) AND v.knowledge_kind = 'claim'
  AND v.candidate_number = 0 AND v.deleted = 0
  AND v.version = (
      SELECT MAX(current.version) FROM knowledge_versions current
      WHERE current.zone_id = v.zone_id
        AND current.knowledge_kind = v.knowledge_kind
        AND current.knowledge_id = v.knowledge_id
        AND current.candidate_number = 0
  )
  AND v.knowledge_id > sqlc.arg(after_claim_id)
ORDER BY v.knowledge_id
LIMIT sqlc.arg(page_size);

-- name: ListCurrentEntities :many
SELECT i.entity_id, i.title, i.entity_type,
       v.version, v.deleted, v.content_hash, v.description
FROM entity_identities i
JOIN knowledge_versions v
  ON v.zone_id = i.zone_id AND v.knowledge_kind = 'entity'
 AND v.knowledge_id = i.entity_id
WHERE i.zone_id = sqlc.arg(zone_id)
  AND i.entity_id > sqlc.arg(after_entity_id)
  AND v.candidate_number = 0
  AND v.version = (
      SELECT MAX(current.version) FROM knowledge_versions current
      WHERE current.zone_id = v.zone_id
        AND current.knowledge_kind = v.knowledge_kind
        AND current.knowledge_id = v.knowledge_id
        AND current.candidate_number = 0
  )
ORDER BY i.entity_id
LIMIT sqlc.arg(page_size);

-- name: ListCurrentRelations :many
SELECT i.relation_id, i.source_entity_id, i.target_entity_id, i.relation_type,
       v.version, v.deleted, v.content_hash, v.description
FROM relation_identities i
JOIN knowledge_versions v
  ON v.zone_id = i.zone_id AND v.knowledge_kind = 'relation'
 AND v.knowledge_id = i.relation_id
WHERE i.zone_id = sqlc.arg(zone_id)
  AND i.relation_id > sqlc.arg(after_relation_id)
  AND v.candidate_number = 0
  AND v.version = (
      SELECT MAX(current.version) FROM knowledge_versions current
      WHERE current.zone_id = v.zone_id
        AND current.knowledge_kind = v.knowledge_kind
        AND current.knowledge_id = v.knowledge_id
        AND current.candidate_number = 0
  )
ORDER BY i.relation_id
LIMIT sqlc.arg(page_size);

-- name: ListCurrentClaims :many
SELECT i.claim_id, i.subject_kind, i.subject_id, i.claim_type,
       v.version, v.deleted, v.content_hash, v.description
FROM claim_identities i
JOIN knowledge_versions v
  ON v.zone_id = i.zone_id AND v.knowledge_kind = 'claim'
 AND v.knowledge_id = i.claim_id
WHERE i.zone_id = sqlc.arg(zone_id)
  AND i.claim_id > sqlc.arg(after_claim_id)
  AND v.candidate_number = 0
  AND v.version = (
      SELECT MAX(current.version) FROM knowledge_versions current
      WHERE current.zone_id = v.zone_id
        AND current.knowledge_kind = v.knowledge_kind
        AND current.knowledge_id = v.knowledge_id
        AND current.candidate_number = 0
  )
ORDER BY i.claim_id
LIMIT sqlc.arg(page_size);

-- name: ListEntityHistory :many
SELECT i.title, i.entity_type, v.version, v.deleted, v.content_hash, v.description
FROM entity_identities i
JOIN knowledge_versions v
  ON v.zone_id = i.zone_id AND v.knowledge_kind = 'entity'
 AND v.knowledge_id = i.entity_id
WHERE i.zone_id = sqlc.arg(zone_id) AND i.entity_id = sqlc.arg(entity_id)
  AND v.candidate_number = 0 AND v.version > sqlc.arg(after_version)
ORDER BY v.version
LIMIT sqlc.arg(page_size);

-- name: ListRelationHistory :many
SELECT i.source_entity_id, i.target_entity_id, i.relation_type,
       v.version, v.deleted, v.content_hash, v.description
FROM relation_identities i
JOIN knowledge_versions v
  ON v.zone_id = i.zone_id AND v.knowledge_kind = 'relation'
 AND v.knowledge_id = i.relation_id
WHERE i.zone_id = sqlc.arg(zone_id) AND i.relation_id = sqlc.arg(relation_id)
  AND v.candidate_number = 0 AND v.version > sqlc.arg(after_version)
ORDER BY v.version
LIMIT sqlc.arg(page_size);

-- name: ListClaimHistory :many
SELECT i.subject_kind, i.subject_id, i.claim_type,
       v.version, v.deleted, v.content_hash, v.description
FROM claim_identities i
JOIN knowledge_versions v
  ON v.zone_id = i.zone_id AND v.knowledge_kind = 'claim'
 AND v.knowledge_id = i.claim_id
WHERE i.zone_id = sqlc.arg(zone_id) AND i.claim_id = sqlc.arg(claim_id)
  AND v.candidate_number = 0 AND v.version > sqlc.arg(after_version)
ORDER BY v.version
LIMIT sqlc.arg(page_size);

-- name: CurrentEntityVersion :one
SELECT MAX(version) FROM knowledge_versions
WHERE zone_id = sqlc.arg(zone_id) AND knowledge_kind = 'entity'
  AND knowledge_id = sqlc.arg(entity_id) AND candidate_number = 0;

-- name: CurrentRelationVersion :one
SELECT MAX(version) FROM knowledge_versions
WHERE zone_id = sqlc.arg(zone_id) AND knowledge_kind = 'relation'
  AND knowledge_id = sqlc.arg(relation_id) AND candidate_number = 0;

-- name: CurrentClaimVersion :one
SELECT MAX(version) FROM knowledge_versions
WHERE zone_id = sqlc.arg(zone_id) AND knowledge_kind = 'claim'
  AND knowledge_id = sqlc.arg(claim_id) AND candidate_number = 0;

-- name: CreateEntityVersion :exec
INSERT INTO knowledge_versions
    (zone_id, knowledge_kind, knowledge_id, version, candidate_number,
     deleted, content_hash, description, created_by_source_id)
VALUES
    (sqlc.arg(zone_id), 'entity', sqlc.arg(entity_id), sqlc.arg(version), 0,
     sqlc.arg(deleted), sqlc.arg(content_hash), sqlc.arg(description),
     sqlc.arg(created_by_source_id));

-- name: CreateEntityAlias :exec
INSERT INTO entity_version_aliases
    (zone_id, entity_id, version, candidate_number, alias)
VALUES
    (sqlc.arg(zone_id), sqlc.arg(entity_id), sqlc.arg(version), 0, sqlc.arg(alias));

-- name: CreateRelationVersion :exec
INSERT INTO knowledge_versions
    (zone_id, knowledge_kind, knowledge_id, version, candidate_number,
     deleted, content_hash, description, created_by_source_id)
VALUES
    (sqlc.arg(zone_id), 'relation', sqlc.arg(relation_id), sqlc.arg(version), 0,
     sqlc.arg(deleted), sqlc.arg(content_hash), sqlc.arg(description),
     sqlc.arg(created_by_source_id));

-- name: CreateClaimVersion :exec
INSERT INTO knowledge_versions
    (zone_id, knowledge_kind, knowledge_id, version, candidate_number,
     deleted, content_hash, description, created_by_source_id)
VALUES
    (sqlc.arg(zone_id), 'claim', sqlc.arg(claim_id), sqlc.arg(version), 0,
     sqlc.arg(deleted), sqlc.arg(content_hash), sqlc.arg(description),
     sqlc.arg(created_by_source_id));

-- name: CountActiveEntity :one
SELECT count(*) FROM knowledge_versions v
WHERE v.zone_id = sqlc.arg(zone_id) AND v.knowledge_kind = 'entity'
  AND v.knowledge_id = sqlc.arg(entity_id)
  AND v.candidate_number = 0 AND v.deleted = 0
  AND v.version = (
      SELECT MAX(current.version) FROM knowledge_versions current
      WHERE current.zone_id = v.zone_id
        AND current.knowledge_kind = v.knowledge_kind
        AND current.knowledge_id = v.knowledge_id
        AND current.candidate_number = 0
  );

-- name: CountActiveRelation :one
SELECT count(*) FROM knowledge_versions v
WHERE v.zone_id = sqlc.arg(zone_id) AND v.knowledge_kind = 'relation'
  AND v.knowledge_id = sqlc.arg(relation_id)
  AND v.candidate_number = 0 AND v.deleted = 0
  AND v.version = (
      SELECT MAX(current.version) FROM knowledge_versions current
      WHERE current.zone_id = v.zone_id
        AND current.knowledge_kind = v.knowledge_kind
        AND current.knowledge_id = v.knowledge_id
        AND current.candidate_number = 0
  );

-- name: ListActiveRelations :many
SELECT i.relation_id FROM relation_identities i
JOIN knowledge_versions v
  ON v.zone_id = i.zone_id AND v.knowledge_kind = 'relation'
 AND v.knowledge_id = i.relation_id AND v.deleted = 0
 AND v.candidate_number = 0
WHERE i.zone_id = sqlc.arg(zone_id)
  AND (i.source_entity_id = sqlc.arg(entity_id) OR i.target_entity_id = sqlc.arg(entity_id))
  AND v.version = (
      SELECT MAX(current.version) FROM knowledge_versions current
      WHERE current.zone_id = v.zone_id
        AND current.knowledge_kind = v.knowledge_kind
        AND current.knowledge_id = v.knowledge_id
        AND current.candidate_number = 0
  )
ORDER BY i.relation_id;

-- name: ListActiveClaims :many
SELECT i.claim_id FROM claim_identities i
JOIN knowledge_versions v
  ON v.zone_id = i.zone_id AND v.knowledge_kind = 'claim'
 AND v.knowledge_id = i.claim_id AND v.deleted = 0
 AND v.candidate_number = 0
WHERE i.zone_id = sqlc.arg(zone_id)
  AND i.subject_kind = sqlc.arg(subject_kind)
  AND i.subject_id = sqlc.arg(subject_id)
  AND v.version = (
      SELECT MAX(current.version) FROM knowledge_versions current
      WHERE current.zone_id = v.zone_id
        AND current.knowledge_kind = v.knowledge_kind
        AND current.knowledge_id = v.knowledge_id
        AND current.candidate_number = 0
  )
ORDER BY i.claim_id;
