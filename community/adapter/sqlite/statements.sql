-- name: GetSchemaVersion :one
SELECT version FROM community_schema WHERE id = 1;

-- name: ValidateSchema :one
SELECT
    (SELECT count(*) FROM community_sets WHERE 0) +
    (SELECT count(*) FROM community_structures WHERE 0) +
	(SELECT count(*) FROM structure_entity_versions WHERE 0) +
	(SELECT count(*) FROM structure_relation_versions WHERE 0) +
	(SELECT count(*) FROM structure_claim_versions WHERE 0) +
    (SELECT count(*) FROM community_structure_state WHERE 0) +
    (SELECT count(*) FROM community_entities WHERE 0) +
    (SELECT count(*) FROM community_relations WHERE 0) +
    (SELECT count(*) FROM community_set_communities WHERE 0) +
    (SELECT count(*) FROM community_members WHERE 0) +
    (SELECT count(*) FROM reports WHERE 0) +
    (SELECT count(*) FROM report_entities WHERE 0) +
    (SELECT count(*) FROM report_relations WHERE 0) +
    (SELECT count(*) FROM report_claim_evidence WHERE 0) +
    (SELECT count(*) FROM report_text_units WHERE 0) +
    (SELECT count(*) FROM report_sets WHERE 0) +
	(SELECT count(*) FROM community_report_publications WHERE 0) +
    (SELECT count(*) FROM report_set_reports WHERE 0) +
    (SELECT count(*) FROM community_schema WHERE 0) +
    (SELECT count(*) FROM community_set_communities INDEXED BY community_set_communities_by_parent WHERE 0) +
    (SELECT count(*) FROM community_structures INDEXED BY community_structures_by_boundary WHERE 0) +
    (SELECT count(*) FROM community_members INDEXED BY community_members_by_entity WHERE 0) +
    (SELECT count(*) FROM reports INDEXED BY reports_by_community WHERE 0) +
    (SELECT count(*) FROM report_sets INDEXED BY report_sets_by_community_set WHERE 0) AS valid;

-- name: CreateStructure :execrows
INSERT OR IGNORE INTO community_structures (
    zone_id, id, community_set_id, corpora_id, knowledge_digest, created_at
) VALUES (
    sqlc.arg(zone_id), sqlc.arg(id), sqlc.arg(community_set_id),
    sqlc.arg(corpora_id), sqlc.arg(knowledge_digest), sqlc.arg(created_at)
);

-- name: GetStructure :one
SELECT id, community_set_id, corpora_id, knowledge_digest, created_at
FROM community_structures
WHERE zone_id = sqlc.arg(zone_id) AND id = sqlc.arg(id);

-- name: GetStructureAt :one
SELECT id, community_set_id, corpora_id, knowledge_digest, created_at
FROM community_structures
WHERE zone_id = sqlc.arg(zone_id)
  AND corpora_id = sqlc.arg(corpora_id)
  AND knowledge_digest = sqlc.arg(knowledge_digest);

-- name: AddStructureEntityVersion :exec
INSERT INTO structure_entity_versions (zone_id, structure_id, entity_id, version)
VALUES (sqlc.arg(zone_id), sqlc.arg(structure_id), sqlc.arg(entity_id), sqlc.arg(version));

-- name: AddStructureRelationVersion :exec
INSERT INTO structure_relation_versions (zone_id, structure_id, relation_id, version)
VALUES (sqlc.arg(zone_id), sqlc.arg(structure_id), sqlc.arg(relation_id), sqlc.arg(version));

-- name: AddStructureClaimVersion :exec
INSERT INTO structure_claim_versions (zone_id, structure_id, claim_id, version)
VALUES (sqlc.arg(zone_id), sqlc.arg(structure_id), sqlc.arg(claim_id), sqlc.arg(version));

-- name: ListStructureEntityVersions :many
SELECT entity_id, version FROM structure_entity_versions
WHERE zone_id = sqlc.arg(zone_id) AND structure_id = sqlc.arg(structure_id)
ORDER BY entity_id;

-- name: ListStructureRelationVersions :many
SELECT relation_id, version FROM structure_relation_versions
WHERE zone_id = sqlc.arg(zone_id) AND structure_id = sqlc.arg(structure_id)
ORDER BY relation_id;

-- name: ListStructureClaimVersions :many
SELECT claim_id, version FROM structure_claim_versions
WHERE zone_id = sqlc.arg(zone_id) AND structure_id = sqlc.arg(structure_id)
ORDER BY claim_id;

-- name: GetStructureState :one
SELECT structure_id, pending_relation_changes
FROM community_structure_state
WHERE zone_id = sqlc.arg(zone_id);

-- name: CreateStructureState :execrows
INSERT OR IGNORE INTO community_structure_state (
    zone_id, structure_id, pending_relation_changes
) VALUES (
    sqlc.arg(zone_id), sqlc.arg(structure_id), sqlc.arg(pending_relation_changes)
);

-- name: AdvanceStructureState :execrows
UPDATE community_structure_state
SET structure_id = sqlc.arg(structure_id),
    pending_relation_changes = sqlc.arg(pending_relation_changes)
WHERE zone_id = sqlc.arg(zone_id)
  AND structure_id = sqlc.arg(expected_structure_id);

-- name: CreateCommunitySet :exec
INSERT INTO community_sets (
    zone_id, id, detector_version, max_cluster_size,
    use_largest_connected_component, seed, created_at
) VALUES (
    sqlc.arg(zone_id), sqlc.arg(id), sqlc.arg(detector_version),
    sqlc.arg(max_cluster_size), sqlc.arg(use_largest_connected_component),
    sqlc.arg(seed), sqlc.arg(created_at)
);

-- name: GetCommunitySet :one
SELECT zone_id, id, detector_version, max_cluster_size,
       use_largest_connected_component, seed, created_at
FROM community_sets
WHERE zone_id = sqlc.arg(zone_id) AND id = sqlc.arg(id);

-- name: AddCommunityEntity :exec
INSERT INTO community_entities (zone_id, community_set_id, entity_id, version)
VALUES (
    sqlc.arg(zone_id), sqlc.arg(community_set_id),
    sqlc.arg(entity_id), sqlc.arg(version)
);

-- name: ListCommunityEntities :many
SELECT entity_id, version FROM community_entities
WHERE zone_id = sqlc.arg(zone_id) AND community_set_id = sqlc.arg(community_set_id)
ORDER BY entity_id;

-- name: AddCommunityRelation :exec
INSERT INTO community_relations (zone_id, community_set_id, relation_id, version)
VALUES (
    sqlc.arg(zone_id), sqlc.arg(community_set_id),
    sqlc.arg(relation_id), sqlc.arg(version)
);

-- name: ListCommunityRelations :many
SELECT relation_id, version FROM community_relations
WHERE zone_id = sqlc.arg(zone_id) AND community_set_id = sqlc.arg(community_set_id)
ORDER BY relation_id;

-- name: AddCommunity :exec
INSERT INTO community_set_communities (
    zone_id, community_set_id, community_id, number, level,
    parent_community_id, final, unsplittable
) VALUES (
    sqlc.arg(zone_id), sqlc.arg(community_set_id), sqlc.arg(community_id),
    sqlc.arg(number), sqlc.arg(level), sqlc.arg(parent_community_id),
    sqlc.arg(final), sqlc.arg(unsplittable)
);

-- name: ListCommunities :many
SELECT community_id, number, level, parent_community_id, final, unsplittable
FROM community_set_communities
WHERE zone_id = sqlc.arg(zone_id) AND community_set_id = sqlc.arg(community_set_id)
ORDER BY level, number;

-- name: AddCommunityMember :exec
INSERT INTO community_members (
    zone_id, community_set_id, community_id, member_ordinal, entity_id
) VALUES (
    sqlc.arg(zone_id), sqlc.arg(community_set_id), sqlc.arg(community_id),
    sqlc.arg(member_ordinal), sqlc.arg(entity_id)
);

-- name: ListCommunityMembers :many
SELECT member_ordinal, entity_id FROM community_members
WHERE zone_id = sqlc.arg(zone_id)
  AND community_set_id = sqlc.arg(community_set_id)
  AND community_id = sqlc.arg(community_id)
ORDER BY member_ordinal;

-- name: FindCommunityWithoutParent :one
SELECT child.community_id
FROM community_set_communities AS child
WHERE child.zone_id = sqlc.arg(zone_id)
  AND child.community_set_id = sqlc.arg(community_set_id)
  AND child.parent_community_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM community_set_communities AS parent
      WHERE parent.zone_id = child.zone_id
        AND parent.community_set_id = child.community_set_id
        AND parent.community_id = child.parent_community_id
  )
LIMIT 1;

-- name: CreateReport :exec
INSERT INTO reports (
    zone_id, id, community_id, period, title, summary, rank,
    rating_explanation, full_content, full_content_json, model,
    prompt_hash, tokenizer, max_input_tokens, max_report_length
) VALUES (
    sqlc.arg(zone_id), sqlc.arg(id), sqlc.arg(community_id), sqlc.arg(period),
    sqlc.arg(title), sqlc.arg(summary), sqlc.arg(rank),
    sqlc.arg(rating_explanation), sqlc.arg(full_content),
    sqlc.arg(full_content_json), sqlc.arg(model), sqlc.arg(prompt_hash),
    sqlc.arg(tokenizer), sqlc.arg(max_input_tokens), sqlc.arg(max_report_length)
);

-- name: GetReport :one
SELECT zone_id, id, community_id, period, title, summary, rank,
       rating_explanation, full_content, full_content_json, model,
       prompt_hash, tokenizer, max_input_tokens, max_report_length
FROM reports
WHERE zone_id = sqlc.arg(zone_id) AND id = sqlc.arg(id);

-- name: AddReportEntity :exec
INSERT INTO report_entities (zone_id, report_id, ordinal, entity_id, version)
VALUES (
    sqlc.arg(zone_id), sqlc.arg(report_id), sqlc.arg(ordinal),
    sqlc.arg(entity_id), sqlc.arg(version)
);

-- name: ListReportEntities :many
SELECT ordinal, entity_id, version FROM report_entities
WHERE zone_id = sqlc.arg(zone_id) AND report_id = sqlc.arg(report_id)
ORDER BY ordinal;

-- name: AddReportRelation :exec
INSERT INTO report_relations (zone_id, report_id, ordinal, relation_id, version)
VALUES (
    sqlc.arg(zone_id), sqlc.arg(report_id), sqlc.arg(ordinal),
    sqlc.arg(relation_id), sqlc.arg(version)
);

-- name: ListReportRelations :many
SELECT ordinal, relation_id, version FROM report_relations
WHERE zone_id = sqlc.arg(zone_id) AND report_id = sqlc.arg(report_id)
ORDER BY ordinal;

-- name: AddReportClaimEvidence :exec
INSERT INTO report_claim_evidence (
    zone_id, report_id, ordinal, claim_id, version, evidence_index
) VALUES (
    sqlc.arg(zone_id), sqlc.arg(report_id), sqlc.arg(ordinal),
    sqlc.arg(claim_id), sqlc.arg(version), sqlc.arg(evidence_index)
);

-- name: ListReportClaimEvidence :many
SELECT ordinal, claim_id, version, evidence_index FROM report_claim_evidence
WHERE zone_id = sqlc.arg(zone_id) AND report_id = sqlc.arg(report_id)
ORDER BY ordinal;

-- name: AddReportTextUnit :exec
INSERT INTO report_text_units (zone_id, report_id, ordinal, text_unit_id)
VALUES (
    sqlc.arg(zone_id), sqlc.arg(report_id),
    sqlc.arg(ordinal), sqlc.arg(text_unit_id)
);

-- name: ListReportTextUnits :many
SELECT ordinal, text_unit_id FROM report_text_units
WHERE zone_id = sqlc.arg(zone_id) AND report_id = sqlc.arg(report_id)
ORDER BY ordinal;

-- name: CreateReportSet :exec
INSERT INTO report_sets (zone_id, id, community_set_id, corpora_id, created_at)
VALUES (
    sqlc.arg(zone_id), sqlc.arg(id), sqlc.arg(community_set_id),
    sqlc.arg(corpora_id), sqlc.arg(created_at)
);

-- name: GetReportPublication :one
SELECT epoch_id, structure_id, report_set_id, vectors_ready, created_at
FROM community_report_publications
WHERE zone_id = sqlc.arg(zone_id) AND structure_id = sqlc.arg(structure_id);

-- name: CreateReportPublication :execrows
INSERT OR IGNORE INTO community_report_publications (
    zone_id, structure_id, epoch_id, report_set_id, vectors_ready, created_at
) VALUES (
    sqlc.arg(zone_id), sqlc.arg(structure_id), sqlc.arg(epoch_id),
    sqlc.arg(report_set_id), 0, sqlc.arg(created_at)
);

-- name: CompleteReportPublication :execrows
UPDATE community_report_publications
SET vectors_ready = 1
WHERE zone_id = sqlc.arg(zone_id)
  AND structure_id = sqlc.arg(structure_id)
  AND report_set_id = sqlc.arg(report_set_id)
  AND vectors_ready = 0;

-- name: GetReportSet :one
SELECT zone_id, id, community_set_id, corpora_id, created_at
FROM report_sets
WHERE zone_id = sqlc.arg(zone_id) AND id = sqlc.arg(id);

-- name: AddReportSetReport :exec
INSERT INTO report_set_reports (zone_id, report_set_id, community_id, report_id)
VALUES (
    sqlc.arg(zone_id), sqlc.arg(report_set_id),
    sqlc.arg(community_id), sqlc.arg(report_id)
);

-- name: ListReportSetReports :many
SELECT community_id, report_id FROM report_set_reports
WHERE zone_id = sqlc.arg(zone_id) AND report_set_id = sqlc.arg(report_set_id)
ORDER BY community_id;

-- name: FindMemberWithoutCommunity :one
SELECT member.entity_id
FROM community_members AS member
WHERE member.zone_id = sqlc.arg(zone_id)
  AND member.community_set_id = sqlc.arg(community_set_id)
  AND NOT EXISTS (
      SELECT 1 FROM community_set_communities AS current
      WHERE current.zone_id = member.zone_id
        AND current.community_set_id = member.community_set_id
        AND current.community_id = member.community_id
  )
LIMIT 1;

-- name: FindMemberWithoutEntity :one
SELECT member.entity_id
FROM community_members AS member
WHERE member.zone_id = sqlc.arg(zone_id)
  AND member.community_set_id = sqlc.arg(community_set_id)
  AND NOT EXISTS (
      SELECT 1 FROM community_entities AS entity
      WHERE entity.zone_id = member.zone_id
        AND entity.community_set_id = member.community_set_id
        AND entity.entity_id = member.entity_id
  )
LIMIT 1;
