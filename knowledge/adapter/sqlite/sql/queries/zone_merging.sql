-- name: SaveZoneEntityConflict :exec
INSERT INTO zone_knowledge_conflicts
    (child_zone_id, knowledge_kind, child_knowledge_id, child_base_version,
     child_candidate_number, parent_zone_id, parent_knowledge_id, parent_version)
SELECT sqlc.arg(child_zone_id), 'entity', sqlc.arg(child_entity_id),
       sqlc.arg(child_base_version), v.candidate_number,
       sqlc.arg(parent_zone_id), sqlc.arg(parent_entity_id), sqlc.arg(parent_version)
FROM knowledge_versions v
WHERE v.zone_id = sqlc.arg(child_zone_id) AND v.knowledge_kind = 'entity'
  AND v.knowledge_id = sqlc.arg(child_entity_id)
  AND v.version = sqlc.arg(child_base_version) AND v.candidate_number > 0
  AND v.content_hash = sqlc.arg(candidate_content_hash)
ON CONFLICT DO NOTHING;

-- name: GetZoneEntityConflict :one
SELECT v.content_hash AS candidate_content_hash, c.parent_zone_id,
       c.parent_knowledge_id AS parent_entity_id, c.parent_version
FROM zone_knowledge_conflicts c
JOIN knowledge_versions v
  ON v.zone_id = c.child_zone_id AND v.knowledge_kind = c.knowledge_kind
 AND v.knowledge_id = c.child_knowledge_id AND v.version = c.child_base_version
 AND v.candidate_number = c.child_candidate_number
WHERE c.child_zone_id = sqlc.arg(child_zone_id) AND c.knowledge_kind = 'entity'
  AND c.child_knowledge_id = sqlc.arg(child_entity_id)
  AND c.child_base_version = sqlc.arg(child_base_version)
  AND c.resolved_by_source_id IS NULL
LIMIT 1;

-- name: ResolveZoneEntityConflict :execrows
UPDATE zone_knowledge_conflicts
SET resolved_by_source_id = sqlc.arg(resolution_source_id)
WHERE child_zone_id = sqlc.arg(child_zone_id) AND knowledge_kind = 'entity'
  AND child_knowledge_id = sqlc.arg(child_entity_id)
  AND child_base_version = sqlc.arg(child_base_version)
  AND child_candidate_number = (
      SELECT candidate_number FROM knowledge_versions
      WHERE zone_id = sqlc.arg(child_zone_id) AND knowledge_kind = 'entity'
        AND knowledge_id = sqlc.arg(child_entity_id)
        AND version = sqlc.arg(child_base_version) AND candidate_number > 0
        AND content_hash = sqlc.arg(candidate_content_hash)
  )
  AND parent_zone_id = sqlc.arg(parent_zone_id)
  AND parent_knowledge_id = sqlc.arg(parent_entity_id)
  AND parent_version = sqlc.arg(parent_version)
  AND resolved_by_source_id IS NULL;

-- name: GetResolvedZoneEntityConflict :one
SELECT c.child_knowledge_id AS child_entity_id, c.child_base_version,
       v.content_hash AS candidate_content_hash, c.parent_zone_id,
       c.parent_knowledge_id AS parent_entity_id, c.parent_version, c.parent_reconciled
FROM zone_knowledge_conflicts c
JOIN knowledge_versions v
  ON v.zone_id = c.child_zone_id AND v.knowledge_kind = c.knowledge_kind
 AND v.knowledge_id = c.child_knowledge_id AND v.version = c.child_base_version
 AND v.candidate_number = c.child_candidate_number
WHERE c.child_zone_id = sqlc.arg(child_zone_id) AND c.knowledge_kind = 'entity'
  AND c.resolved_by_source_id = sqlc.arg(resolution_source_id)
LIMIT 1;

-- name: ReconcileZoneEntityConflict :execrows
UPDATE zone_knowledge_conflicts SET parent_reconciled = 1
WHERE child_zone_id = sqlc.arg(child_zone_id) AND knowledge_kind = 'entity'
  AND child_knowledge_id = sqlc.arg(child_entity_id)
  AND child_base_version = sqlc.arg(child_base_version)
  AND child_candidate_number = (
      SELECT candidate_number FROM knowledge_versions
      WHERE zone_id = sqlc.arg(child_zone_id) AND knowledge_kind = 'entity'
        AND knowledge_id = sqlc.arg(child_entity_id)
        AND version = sqlc.arg(child_base_version) AND candidate_number > 0
        AND content_hash = sqlc.arg(candidate_content_hash)
  )
  AND parent_zone_id = sqlc.arg(parent_zone_id)
  AND parent_knowledge_id = sqlc.arg(parent_entity_id)
  AND parent_version = sqlc.arg(parent_version)
  AND zone_knowledge_conflicts.resolved_by_source_id = sqlc.arg(resolution_source_id)
  AND parent_reconciled = 0;

-- name: SaveZoneRelationConflict :exec
INSERT INTO zone_knowledge_conflicts
    (child_zone_id, knowledge_kind, child_knowledge_id, child_base_version,
     child_candidate_number, parent_zone_id, parent_knowledge_id, parent_version)
SELECT sqlc.arg(child_zone_id), 'relation', sqlc.arg(child_relation_id),
       sqlc.arg(child_base_version), v.candidate_number,
       sqlc.arg(parent_zone_id), sqlc.arg(parent_relation_id), sqlc.arg(parent_version)
FROM knowledge_versions v
WHERE v.zone_id = sqlc.arg(child_zone_id) AND v.knowledge_kind = 'relation'
  AND v.knowledge_id = sqlc.arg(child_relation_id)
  AND v.version = sqlc.arg(child_base_version) AND v.candidate_number > 0
  AND v.content_hash = sqlc.arg(candidate_content_hash)
ON CONFLICT DO NOTHING;

-- name: GetZoneRelationConflict :one
SELECT v.content_hash AS candidate_content_hash, c.parent_zone_id,
       c.parent_knowledge_id AS parent_relation_id, c.parent_version
FROM zone_knowledge_conflicts c
JOIN knowledge_versions v
  ON v.zone_id = c.child_zone_id AND v.knowledge_kind = c.knowledge_kind
 AND v.knowledge_id = c.child_knowledge_id AND v.version = c.child_base_version
 AND v.candidate_number = c.child_candidate_number
WHERE c.child_zone_id = sqlc.arg(child_zone_id) AND c.knowledge_kind = 'relation'
  AND c.child_knowledge_id = sqlc.arg(child_relation_id)
  AND c.child_base_version = sqlc.arg(child_base_version)
  AND c.resolved_by_source_id IS NULL
LIMIT 1;

-- name: ResolveZoneRelationConflict :execrows
UPDATE zone_knowledge_conflicts
SET resolved_by_source_id = sqlc.arg(resolution_source_id)
WHERE child_zone_id = sqlc.arg(child_zone_id) AND knowledge_kind = 'relation'
  AND child_knowledge_id = sqlc.arg(child_relation_id)
  AND child_base_version = sqlc.arg(child_base_version)
  AND child_candidate_number = (
      SELECT candidate_number FROM knowledge_versions
      WHERE zone_id = sqlc.arg(child_zone_id) AND knowledge_kind = 'relation'
        AND knowledge_id = sqlc.arg(child_relation_id)
        AND version = sqlc.arg(child_base_version) AND candidate_number > 0
        AND content_hash = sqlc.arg(candidate_content_hash)
  )
  AND parent_zone_id = sqlc.arg(parent_zone_id)
  AND parent_knowledge_id = sqlc.arg(parent_relation_id)
  AND parent_version = sqlc.arg(parent_version)
  AND resolved_by_source_id IS NULL;

-- name: GetResolvedZoneRelationConflict :one
SELECT c.child_knowledge_id AS child_relation_id, c.child_base_version,
       v.content_hash AS candidate_content_hash, c.parent_zone_id,
       c.parent_knowledge_id AS parent_relation_id, c.parent_version, c.parent_reconciled
FROM zone_knowledge_conflicts c
JOIN knowledge_versions v
  ON v.zone_id = c.child_zone_id AND v.knowledge_kind = c.knowledge_kind
 AND v.knowledge_id = c.child_knowledge_id AND v.version = c.child_base_version
 AND v.candidate_number = c.child_candidate_number
WHERE c.child_zone_id = sqlc.arg(child_zone_id) AND c.knowledge_kind = 'relation'
  AND c.resolved_by_source_id = sqlc.arg(resolution_source_id)
LIMIT 1;

-- name: ReconcileZoneRelationConflict :execrows
UPDATE zone_knowledge_conflicts SET parent_reconciled = 1
WHERE child_zone_id = sqlc.arg(child_zone_id) AND knowledge_kind = 'relation'
  AND child_knowledge_id = sqlc.arg(child_relation_id)
  AND child_base_version = sqlc.arg(child_base_version)
  AND child_candidate_number = (
      SELECT candidate_number FROM knowledge_versions
      WHERE zone_id = sqlc.arg(child_zone_id) AND knowledge_kind = 'relation'
        AND knowledge_id = sqlc.arg(child_relation_id)
        AND version = sqlc.arg(child_base_version) AND candidate_number > 0
        AND content_hash = sqlc.arg(candidate_content_hash)
  )
  AND parent_zone_id = sqlc.arg(parent_zone_id)
  AND parent_knowledge_id = sqlc.arg(parent_relation_id)
  AND parent_version = sqlc.arg(parent_version)
  AND zone_knowledge_conflicts.resolved_by_source_id = sqlc.arg(resolution_source_id)
  AND parent_reconciled = 0;

-- name: SaveZoneClaimConflict :exec
INSERT INTO zone_knowledge_conflicts
    (child_zone_id, knowledge_kind, child_knowledge_id, child_base_version,
     child_candidate_number, parent_zone_id, parent_knowledge_id, parent_version)
SELECT sqlc.arg(child_zone_id), 'claim', sqlc.arg(child_claim_id),
       sqlc.arg(child_base_version), v.candidate_number,
       sqlc.arg(parent_zone_id), sqlc.arg(parent_claim_id), sqlc.arg(parent_version)
FROM knowledge_versions v
WHERE v.zone_id = sqlc.arg(child_zone_id) AND v.knowledge_kind = 'claim'
  AND v.knowledge_id = sqlc.arg(child_claim_id)
  AND v.version = sqlc.arg(child_base_version) AND v.candidate_number > 0
  AND v.content_hash = sqlc.arg(candidate_content_hash)
ON CONFLICT DO NOTHING;

-- name: GetZoneClaimConflict :one
SELECT v.content_hash AS candidate_content_hash, c.parent_zone_id,
       c.parent_knowledge_id AS parent_claim_id, c.parent_version
FROM zone_knowledge_conflicts c
JOIN knowledge_versions v
  ON v.zone_id = c.child_zone_id AND v.knowledge_kind = c.knowledge_kind
 AND v.knowledge_id = c.child_knowledge_id AND v.version = c.child_base_version
 AND v.candidate_number = c.child_candidate_number
WHERE c.child_zone_id = sqlc.arg(child_zone_id) AND c.knowledge_kind = 'claim'
  AND c.child_knowledge_id = sqlc.arg(child_claim_id)
  AND c.child_base_version = sqlc.arg(child_base_version)
  AND c.resolved_by_source_id IS NULL
LIMIT 1;

-- name: ResolveZoneClaimConflict :execrows
UPDATE zone_knowledge_conflicts
SET resolved_by_source_id = sqlc.arg(resolution_source_id)
WHERE child_zone_id = sqlc.arg(child_zone_id) AND knowledge_kind = 'claim'
  AND child_knowledge_id = sqlc.arg(child_claim_id)
  AND child_base_version = sqlc.arg(child_base_version)
  AND child_candidate_number = (
      SELECT candidate_number FROM knowledge_versions
      WHERE zone_id = sqlc.arg(child_zone_id) AND knowledge_kind = 'claim'
        AND knowledge_id = sqlc.arg(child_claim_id)
        AND version = sqlc.arg(child_base_version) AND candidate_number > 0
        AND content_hash = sqlc.arg(candidate_content_hash)
  )
  AND parent_zone_id = sqlc.arg(parent_zone_id)
  AND parent_knowledge_id = sqlc.arg(parent_claim_id)
  AND parent_version = sqlc.arg(parent_version)
  AND resolved_by_source_id IS NULL;

-- name: GetResolvedZoneClaimConflict :one
SELECT c.child_knowledge_id AS child_claim_id, c.child_base_version,
       v.content_hash AS candidate_content_hash, c.parent_zone_id,
       c.parent_knowledge_id AS parent_claim_id, c.parent_version, c.parent_reconciled
FROM zone_knowledge_conflicts c
JOIN knowledge_versions v
  ON v.zone_id = c.child_zone_id AND v.knowledge_kind = c.knowledge_kind
 AND v.knowledge_id = c.child_knowledge_id AND v.version = c.child_base_version
 AND v.candidate_number = c.child_candidate_number
WHERE c.child_zone_id = sqlc.arg(child_zone_id) AND c.knowledge_kind = 'claim'
  AND c.resolved_by_source_id = sqlc.arg(resolution_source_id)
LIMIT 1;

-- name: ReconcileZoneClaimConflict :execrows
UPDATE zone_knowledge_conflicts SET parent_reconciled = 1
WHERE child_zone_id = sqlc.arg(child_zone_id) AND knowledge_kind = 'claim'
  AND child_knowledge_id = sqlc.arg(child_claim_id)
  AND child_base_version = sqlc.arg(child_base_version)
  AND child_candidate_number = (
      SELECT candidate_number FROM knowledge_versions
      WHERE zone_id = sqlc.arg(child_zone_id) AND knowledge_kind = 'claim'
        AND knowledge_id = sqlc.arg(child_claim_id)
        AND version = sqlc.arg(child_base_version) AND candidate_number > 0
        AND content_hash = sqlc.arg(candidate_content_hash)
  )
  AND parent_zone_id = sqlc.arg(parent_zone_id)
  AND parent_knowledge_id = sqlc.arg(parent_claim_id)
  AND parent_version = sqlc.arg(parent_version)
  AND zone_knowledge_conflicts.resolved_by_source_id = sqlc.arg(resolution_source_id)
  AND parent_reconciled = 0;
