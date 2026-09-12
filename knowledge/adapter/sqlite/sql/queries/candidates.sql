-- name: GetEntityCandidate :one
SELECT candidate_number, description FROM knowledge_versions
WHERE zone_id = sqlc.arg(zone_id) AND knowledge_kind = 'entity'
  AND knowledge_id = sqlc.arg(entity_id) AND version = sqlc.arg(base_version)
  AND candidate_number > 0 AND content_hash = sqlc.arg(content_hash)
  AND resolved_by_source_id IS NULL;

-- name: ListEntityCandidateAliases :many
SELECT a.alias FROM entity_version_aliases a
WHERE a.zone_id = sqlc.arg(zone_id) AND a.entity_id = sqlc.arg(entity_id)
  AND a.version = sqlc.arg(base_version)
  AND a.candidate_number = sqlc.arg(candidate_number)
ORDER BY a.alias;

-- name: NextEntityCandidateNumber :one
SELECT COALESCE(MAX(candidate_number), 0) + 1 FROM knowledge_versions
WHERE zone_id = sqlc.arg(zone_id) AND knowledge_kind = 'entity'
  AND knowledge_id = sqlc.arg(entity_id) AND version = sqlc.arg(base_version);

-- name: CreateEntityCandidate :exec
INSERT INTO knowledge_versions
    (zone_id, knowledge_kind, knowledge_id, version, candidate_number,
     deleted, content_hash, description, created_by_source_id)
VALUES
    (sqlc.arg(zone_id), 'entity', sqlc.arg(entity_id), sqlc.arg(base_version),
     sqlc.arg(candidate_number), 0, sqlc.arg(content_hash), sqlc.arg(description),
     sqlc.arg(created_by_source_id));

-- name: CreateEntityCandidateAlias :exec
INSERT INTO entity_version_aliases
    (zone_id, entity_id, version, candidate_number, alias)
VALUES
    (sqlc.arg(zone_id), sqlc.arg(entity_id), sqlc.arg(base_version),
     sqlc.arg(candidate_number), sqlc.arg(alias));

-- name: ListEntityCandidates :many
SELECT content_hash, candidate_number, description FROM knowledge_versions
WHERE zone_id = sqlc.arg(zone_id) AND knowledge_kind = 'entity'
  AND knowledge_id = sqlc.arg(entity_id) AND version = sqlc.arg(base_version)
  AND candidate_number > 0 AND resolved_by_source_id IS NULL
ORDER BY candidate_number;

-- name: GetRelationCandidate :one
SELECT candidate_number, description FROM knowledge_versions
WHERE zone_id = sqlc.arg(zone_id) AND knowledge_kind = 'relation'
  AND knowledge_id = sqlc.arg(relation_id) AND version = sqlc.arg(base_version)
  AND candidate_number > 0 AND content_hash = sqlc.arg(content_hash)
  AND resolved_by_source_id IS NULL;

-- name: NextRelationCandidateNumber :one
SELECT COALESCE(MAX(candidate_number), 0) + 1 FROM knowledge_versions
WHERE zone_id = sqlc.arg(zone_id) AND knowledge_kind = 'relation'
  AND knowledge_id = sqlc.arg(relation_id) AND version = sqlc.arg(base_version);

-- name: CreateRelationCandidate :exec
INSERT INTO knowledge_versions
    (zone_id, knowledge_kind, knowledge_id, version, candidate_number,
     deleted, content_hash, description, created_by_source_id)
VALUES
    (sqlc.arg(zone_id), 'relation', sqlc.arg(relation_id), sqlc.arg(base_version),
     sqlc.arg(candidate_number), 0, sqlc.arg(content_hash), sqlc.arg(description),
     sqlc.arg(created_by_source_id));

-- name: ListRelationCandidates :many
SELECT content_hash, candidate_number, description FROM knowledge_versions
WHERE zone_id = sqlc.arg(zone_id) AND knowledge_kind = 'relation'
  AND knowledge_id = sqlc.arg(relation_id) AND version = sqlc.arg(base_version)
  AND candidate_number > 0 AND resolved_by_source_id IS NULL
ORDER BY candidate_number;

-- name: GetClaimCandidate :one
SELECT candidate_number, description FROM knowledge_versions
WHERE zone_id = sqlc.arg(zone_id) AND knowledge_kind = 'claim'
  AND knowledge_id = sqlc.arg(claim_id) AND version = sqlc.arg(base_version)
  AND candidate_number > 0 AND content_hash = sqlc.arg(content_hash)
  AND resolved_by_source_id IS NULL;

-- name: NextClaimCandidateNumber :one
SELECT COALESCE(MAX(candidate_number), 0) + 1 FROM knowledge_versions
WHERE zone_id = sqlc.arg(zone_id) AND knowledge_kind = 'claim'
  AND knowledge_id = sqlc.arg(claim_id) AND version = sqlc.arg(base_version);

-- name: CreateClaimCandidate :exec
INSERT INTO knowledge_versions
    (zone_id, knowledge_kind, knowledge_id, version, candidate_number,
     deleted, content_hash, description, created_by_source_id)
VALUES
    (sqlc.arg(zone_id), 'claim', sqlc.arg(claim_id), sqlc.arg(base_version),
     sqlc.arg(candidate_number), 0, sqlc.arg(content_hash), sqlc.arg(description),
     sqlc.arg(created_by_source_id));

-- name: ListClaimCandidates :many
SELECT content_hash, candidate_number, description FROM knowledge_versions
WHERE zone_id = sqlc.arg(zone_id) AND knowledge_kind = 'claim'
  AND knowledge_id = sqlc.arg(claim_id) AND version = sqlc.arg(base_version)
  AND candidate_number > 0 AND resolved_by_source_id IS NULL
ORDER BY candidate_number;

-- name: BrowsePendingEntityIDs :many
SELECT DISTINCT v.knowledge_id AS entity_id
FROM knowledge_versions v
WHERE v.zone_id = sqlc.arg(zone_id)
  AND v.knowledge_kind = 'entity'
  AND v.knowledge_id > sqlc.arg(after_id)
  AND v.candidate_number > 0
  AND v.resolved_by_source_id IS NULL
ORDER BY v.knowledge_id
LIMIT sqlc.arg(page_limit);

-- name: ResolveEntityCandidate :execrows
UPDATE knowledge_versions SET resolved_by_source_id = sqlc.arg(resolution_source_id)
WHERE zone_id = sqlc.arg(zone_id) AND knowledge_kind = 'entity'
  AND knowledge_id = sqlc.arg(entity_id) AND version = sqlc.arg(base_version)
  AND candidate_number > 0 AND content_hash = sqlc.arg(content_hash)
  AND resolved_by_source_id IS NULL;

-- name: ResolveRelationCandidate :execrows
UPDATE knowledge_versions SET resolved_by_source_id = sqlc.arg(resolution_source_id)
WHERE zone_id = sqlc.arg(zone_id) AND knowledge_kind = 'relation'
  AND knowledge_id = sqlc.arg(relation_id) AND version = sqlc.arg(base_version)
  AND candidate_number > 0 AND content_hash = sqlc.arg(content_hash)
  AND resolved_by_source_id IS NULL;

-- name: ResolveClaimCandidate :execrows
UPDATE knowledge_versions SET resolved_by_source_id = sqlc.arg(resolution_source_id)
WHERE zone_id = sqlc.arg(zone_id) AND knowledge_kind = 'claim'
  AND knowledge_id = sqlc.arg(claim_id) AND version = sqlc.arg(base_version)
  AND candidate_number > 0 AND content_hash = sqlc.arg(content_hash)
  AND resolved_by_source_id IS NULL;

-- name: BrowsePendingRelationIDs :many
SELECT DISTINCT v.knowledge_id AS relation_id
FROM knowledge_versions v
WHERE v.zone_id = sqlc.arg(zone_id)
  AND v.knowledge_kind = 'relation'
  AND v.knowledge_id > sqlc.arg(after_id)
  AND v.candidate_number > 0
  AND v.resolved_by_source_id IS NULL
ORDER BY v.knowledge_id
LIMIT sqlc.arg(page_limit);

-- name: BrowsePendingClaimIDs :many
SELECT DISTINCT v.knowledge_id AS claim_id
FROM knowledge_versions v
WHERE v.zone_id = sqlc.arg(zone_id)
  AND v.knowledge_kind = 'claim'
  AND v.knowledge_id > sqlc.arg(after_id)
  AND v.candidate_number > 0
  AND v.resolved_by_source_id IS NULL
ORDER BY v.knowledge_id
LIMIT sqlc.arg(page_limit);
