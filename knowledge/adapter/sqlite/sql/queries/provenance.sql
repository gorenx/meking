-- name: GetKnowledgeSource :one
SELECT source_id, source_kind, producer_id FROM knowledge_sources
WHERE zone_id = sqlc.arg(zone_id) AND source_id = sqlc.arg(source_id);

-- name: CreateKnowledgeSource :exec
INSERT INTO knowledge_sources (zone_id, source_id, source_kind, producer_id)
VALUES
    (sqlc.arg(zone_id), sqlc.arg(source_id), sqlc.arg(source_kind), sqlc.arg(producer_id));

-- name: ListEntityVersionSources :many
SELECT m.source_id, m.metadata_json FROM knowledge_source_metadata m
WHERE m.zone_id = sqlc.arg(zone_id) AND m.knowledge_kind = 'entity'
  AND m.knowledge_id = sqlc.arg(entity_id) AND m.version = sqlc.arg(version)
  AND m.candidate_number = 0
UNION
SELECT candidate_source.source_id, candidate_source.metadata_json
FROM knowledge_versions formal
JOIN knowledge_versions candidate
  ON candidate.zone_id = formal.zone_id
 AND candidate.knowledge_kind = formal.knowledge_kind
 AND candidate.knowledge_id = formal.knowledge_id
 AND candidate.version = formal.version - 1
 AND candidate.candidate_number > 0
 AND candidate.content_hash = formal.content_hash
 AND candidate.resolved_by_source_id = formal.created_by_source_id
JOIN knowledge_source_metadata candidate_source
  ON candidate_source.zone_id = candidate.zone_id
 AND candidate_source.knowledge_kind = candidate.knowledge_kind
 AND candidate_source.knowledge_id = candidate.knowledge_id
 AND candidate_source.version = candidate.version
 AND candidate_source.candidate_number = candidate.candidate_number
WHERE formal.zone_id = sqlc.arg(zone_id) AND formal.knowledge_kind = 'entity'
  AND formal.knowledge_id = sqlc.arg(entity_id) AND formal.version = sqlc.arg(version)
  AND formal.candidate_number = 0
ORDER BY source_id;

-- name: ListRelationVersionSources :many
SELECT m.source_id, m.metadata_json FROM knowledge_source_metadata m
WHERE m.zone_id = sqlc.arg(zone_id) AND m.knowledge_kind = 'relation'
  AND m.knowledge_id = sqlc.arg(relation_id) AND m.version = sqlc.arg(version)
  AND m.candidate_number = 0
UNION
SELECT candidate_source.source_id, candidate_source.metadata_json
FROM knowledge_versions formal
JOIN knowledge_versions candidate
  ON candidate.zone_id = formal.zone_id
 AND candidate.knowledge_kind = formal.knowledge_kind
 AND candidate.knowledge_id = formal.knowledge_id
 AND candidate.version = formal.version - 1
 AND candidate.candidate_number > 0
 AND candidate.content_hash = formal.content_hash
 AND candidate.resolved_by_source_id = formal.created_by_source_id
JOIN knowledge_source_metadata candidate_source
  ON candidate_source.zone_id = candidate.zone_id
 AND candidate_source.knowledge_kind = candidate.knowledge_kind
 AND candidate_source.knowledge_id = candidate.knowledge_id
 AND candidate_source.version = candidate.version
 AND candidate_source.candidate_number = candidate.candidate_number
WHERE formal.zone_id = sqlc.arg(zone_id) AND formal.knowledge_kind = 'relation'
  AND formal.knowledge_id = sqlc.arg(relation_id) AND formal.version = sqlc.arg(version)
  AND formal.candidate_number = 0
ORDER BY source_id;

-- name: ListClaimVersionSources :many
SELECT m.source_id, m.metadata_json FROM knowledge_source_metadata m
WHERE m.zone_id = sqlc.arg(zone_id) AND m.knowledge_kind = 'claim'
  AND m.knowledge_id = sqlc.arg(claim_id) AND m.version = sqlc.arg(version)
  AND m.candidate_number = 0
UNION
SELECT candidate_source.source_id, candidate_source.metadata_json
FROM knowledge_versions formal
JOIN knowledge_versions candidate
  ON candidate.zone_id = formal.zone_id
 AND candidate.knowledge_kind = formal.knowledge_kind
 AND candidate.knowledge_id = formal.knowledge_id
 AND candidate.version = formal.version - 1
 AND candidate.candidate_number > 0
 AND candidate.content_hash = formal.content_hash
 AND candidate.resolved_by_source_id = formal.created_by_source_id
JOIN knowledge_source_metadata candidate_source
  ON candidate_source.zone_id = candidate.zone_id
 AND candidate_source.knowledge_kind = candidate.knowledge_kind
 AND candidate_source.knowledge_id = candidate.knowledge_id
 AND candidate_source.version = candidate.version
 AND candidate_source.candidate_number = candidate.candidate_number
WHERE formal.zone_id = sqlc.arg(zone_id) AND formal.knowledge_kind = 'claim'
  AND formal.knowledge_id = sqlc.arg(claim_id) AND formal.version = sqlc.arg(version)
  AND formal.candidate_number = 0
ORDER BY source_id;

-- name: ListEntityCandidateSources :many
SELECT m.source_id, m.metadata_json, v.created_by_source_id = m.source_id AS opened_candidate
FROM knowledge_source_metadata m
JOIN knowledge_versions v
  ON v.zone_id = m.zone_id AND v.knowledge_kind = m.knowledge_kind
 AND v.knowledge_id = m.knowledge_id AND v.version = m.version
 AND v.candidate_number = m.candidate_number
WHERE m.zone_id = sqlc.arg(zone_id) AND m.knowledge_kind = 'entity'
  AND m.knowledge_id = sqlc.arg(entity_id) AND m.version = sqlc.arg(base_version)
  AND m.candidate_number > 0 AND v.content_hash = sqlc.arg(content_hash)
  AND v.resolved_by_source_id IS NULL
ORDER BY m.source_id;

-- name: ListRelationCandidateSources :many
SELECT m.source_id, m.metadata_json, v.created_by_source_id = m.source_id AS opened_candidate
FROM knowledge_source_metadata m
JOIN knowledge_versions v
  ON v.zone_id = m.zone_id AND v.knowledge_kind = m.knowledge_kind
 AND v.knowledge_id = m.knowledge_id AND v.version = m.version
 AND v.candidate_number = m.candidate_number
WHERE m.zone_id = sqlc.arg(zone_id) AND m.knowledge_kind = 'relation'
  AND m.knowledge_id = sqlc.arg(relation_id) AND m.version = sqlc.arg(base_version)
  AND m.candidate_number > 0 AND v.content_hash = sqlc.arg(content_hash)
  AND v.resolved_by_source_id IS NULL
ORDER BY m.source_id;

-- name: ListClaimCandidateSources :many
SELECT m.source_id, m.metadata_json, v.created_by_source_id = m.source_id AS opened_candidate
FROM knowledge_source_metadata m
JOIN knowledge_versions v
  ON v.zone_id = m.zone_id AND v.knowledge_kind = m.knowledge_kind
 AND v.knowledge_id = m.knowledge_id AND v.version = m.version
 AND v.candidate_number = m.candidate_number
WHERE m.zone_id = sqlc.arg(zone_id) AND m.knowledge_kind = 'claim'
  AND m.knowledge_id = sqlc.arg(claim_id) AND m.version = sqlc.arg(base_version)
  AND m.candidate_number > 0 AND v.content_hash = sqlc.arg(content_hash)
  AND v.resolved_by_source_id IS NULL
ORDER BY m.source_id;

-- name: SaveKnowledgeSourceMetadata :exec
INSERT INTO knowledge_source_metadata
    (zone_id, source_id, knowledge_kind, knowledge_id, version,
     candidate_number, metadata_json)
VALUES
    (sqlc.arg(zone_id), sqlc.arg(source_id), sqlc.arg(knowledge_kind),
     sqlc.arg(knowledge_id), sqlc.arg(version), sqlc.arg(candidate_number),
     sqlc.arg(metadata_json));

-- name: GetKnowledgeSourceMetadata :one
SELECT metadata_json FROM knowledge_source_metadata
WHERE zone_id = sqlc.arg(zone_id) AND source_id = sqlc.arg(source_id)
  AND knowledge_kind = sqlc.arg(knowledge_kind)
  AND knowledge_id = sqlc.arg(knowledge_id)
  AND version = sqlc.arg(version)
  AND candidate_number = sqlc.arg(candidate_number);

-- name: SaveKnowledgeEvidence :exec
INSERT INTO knowledge_evidence (zone_id, source_id, evidence_json)
VALUES (sqlc.arg(zone_id), sqlc.arg(source_id), sqlc.arg(evidence_json));

-- name: GetKnowledgeEvidence :one
SELECT evidence_json FROM knowledge_evidence
WHERE zone_id = sqlc.arg(zone_id) AND source_id = sqlc.arg(source_id);

-- name: ListSourceEntityVersions :many
SELECT m.knowledge_id AS entity_id, m.version,
       v.created_by_source_id = m.source_id AS created_version
FROM knowledge_source_metadata m
JOIN knowledge_versions v
  ON v.zone_id = m.zone_id AND v.knowledge_kind = m.knowledge_kind
 AND v.knowledge_id = m.knowledge_id AND v.version = m.version
 AND v.candidate_number = m.candidate_number
WHERE m.zone_id = sqlc.arg(zone_id) AND m.knowledge_kind = 'entity'
  AND m.source_id = sqlc.arg(source_id) AND m.candidate_number = 0
ORDER BY m.knowledge_id, m.version;

-- name: ListSourceRelationVersions :many
SELECT m.knowledge_id AS relation_id, m.version,
       v.created_by_source_id = m.source_id AS created_version
FROM knowledge_source_metadata m
JOIN knowledge_versions v
  ON v.zone_id = m.zone_id AND v.knowledge_kind = m.knowledge_kind
 AND v.knowledge_id = m.knowledge_id AND v.version = m.version
 AND v.candidate_number = m.candidate_number
WHERE m.zone_id = sqlc.arg(zone_id) AND m.knowledge_kind = 'relation'
  AND m.source_id = sqlc.arg(source_id) AND m.candidate_number = 0
ORDER BY m.knowledge_id, m.version;

-- name: ListSourceClaimVersions :many
SELECT m.knowledge_id AS claim_id, m.version,
       v.created_by_source_id = m.source_id AS created_version
FROM knowledge_source_metadata m
JOIN knowledge_versions v
  ON v.zone_id = m.zone_id AND v.knowledge_kind = m.knowledge_kind
 AND v.knowledge_id = m.knowledge_id AND v.version = m.version
 AND v.candidate_number = m.candidate_number
WHERE m.zone_id = sqlc.arg(zone_id) AND m.knowledge_kind = 'claim'
  AND m.source_id = sqlc.arg(source_id) AND m.candidate_number = 0
ORDER BY m.knowledge_id, m.version;

-- name: ListSourceEntityCandidates :many
SELECT m.knowledge_id AS entity_id, m.version AS base_version, v.content_hash,
       v.created_by_source_id = m.source_id AS opened_candidate
FROM knowledge_source_metadata m
JOIN knowledge_versions v
  ON v.zone_id = m.zone_id AND v.knowledge_kind = m.knowledge_kind
 AND v.knowledge_id = m.knowledge_id AND v.version = m.version
 AND v.candidate_number = m.candidate_number
WHERE m.zone_id = sqlc.arg(zone_id) AND m.knowledge_kind = 'entity'
  AND m.source_id = sqlc.arg(source_id) AND m.candidate_number > 0
ORDER BY m.knowledge_id, m.version, v.content_hash;

-- name: ListSourceRelationCandidates :many
SELECT m.knowledge_id AS relation_id, m.version AS base_version, v.content_hash,
       v.created_by_source_id = m.source_id AS opened_candidate
FROM knowledge_source_metadata m
JOIN knowledge_versions v
  ON v.zone_id = m.zone_id AND v.knowledge_kind = m.knowledge_kind
 AND v.knowledge_id = m.knowledge_id AND v.version = m.version
 AND v.candidate_number = m.candidate_number
WHERE m.zone_id = sqlc.arg(zone_id) AND m.knowledge_kind = 'relation'
  AND m.source_id = sqlc.arg(source_id) AND m.candidate_number > 0
ORDER BY m.knowledge_id, m.version, v.content_hash;

-- name: ListSourceClaimCandidates :many
SELECT m.knowledge_id AS claim_id, m.version AS base_version, v.content_hash,
       v.created_by_source_id = m.source_id AS opened_candidate
FROM knowledge_source_metadata m
JOIN knowledge_versions v
  ON v.zone_id = m.zone_id AND v.knowledge_kind = m.knowledge_kind
 AND v.knowledge_id = m.knowledge_id AND v.version = m.version
 AND v.candidate_number = m.candidate_number
WHERE m.zone_id = sqlc.arg(zone_id) AND m.knowledge_kind = 'claim'
  AND m.source_id = sqlc.arg(source_id) AND m.candidate_number > 0
ORDER BY m.knowledge_id, m.version, v.content_hash;
