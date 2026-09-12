CREATE TABLE community_schema (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    version INTEGER NOT NULL CHECK (version > 0)
) STRICT;

CREATE TABLE community_sets (
    zone_id TEXT NOT NULL,
    id TEXT NOT NULL,
    detector_version INTEGER NOT NULL CHECK (detector_version > 0),
    max_cluster_size INTEGER NOT NULL CHECK (max_cluster_size > 0),
    use_largest_connected_component INTEGER NOT NULL
        CHECK (use_largest_connected_component IN (0, 1)),
    seed INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (zone_id, id)
) STRICT;

CREATE TABLE community_structures (
    zone_id TEXT NOT NULL,
    id TEXT NOT NULL,
    community_set_id TEXT NOT NULL,
    corpora_id TEXT NOT NULL CHECK (length(trim(corpora_id)) > 0),
    knowledge_digest TEXT NOT NULL CHECK (length(knowledge_digest) = 64),
    created_at TEXT NOT NULL CHECK (length(trim(created_at)) > 0),
    PRIMARY KEY (zone_id, id)
) STRICT;

CREATE UNIQUE INDEX community_structures_by_boundary
    ON community_structures(zone_id, corpora_id, knowledge_digest);

CREATE TABLE structure_entity_versions (
    zone_id TEXT NOT NULL,
    structure_id TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    PRIMARY KEY (zone_id, structure_id, entity_id)
) STRICT;

CREATE TABLE structure_relation_versions (
    zone_id TEXT NOT NULL,
    structure_id TEXT NOT NULL,
    relation_id TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    PRIMARY KEY (zone_id, structure_id, relation_id)
) STRICT;

CREATE TABLE structure_claim_versions (
    zone_id TEXT NOT NULL,
    structure_id TEXT NOT NULL,
    claim_id TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    PRIMARY KEY (zone_id, structure_id, claim_id)
) STRICT;

CREATE TABLE community_structure_state (
    zone_id TEXT PRIMARY KEY,
    structure_id TEXT NOT NULL,
    pending_relation_changes INTEGER NOT NULL CHECK (pending_relation_changes >= 0)
) STRICT;

CREATE TABLE community_entities (
    zone_id TEXT NOT NULL,
    community_set_id TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    PRIMARY KEY (zone_id, community_set_id, entity_id)
) STRICT;

CREATE TABLE community_relations (
    zone_id TEXT NOT NULL,
    community_set_id TEXT NOT NULL,
    relation_id TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    PRIMARY KEY (zone_id, community_set_id, relation_id)
) STRICT;

CREATE TABLE community_set_communities (
    zone_id TEXT NOT NULL,
    community_set_id TEXT NOT NULL,
    community_id TEXT NOT NULL,
    number INTEGER NOT NULL CHECK (number >= 0),
    level INTEGER NOT NULL CHECK (level >= 0),
    parent_community_id TEXT,
    final INTEGER NOT NULL CHECK (final IN (0, 1)),
    unsplittable INTEGER NOT NULL CHECK (unsplittable IN (0, 1)),
    PRIMARY KEY (zone_id, community_set_id, community_id),
    UNIQUE (zone_id, community_set_id, number)
) STRICT;

CREATE INDEX community_set_communities_by_parent
    ON community_set_communities(zone_id, community_set_id, parent_community_id);

CREATE TABLE community_members (
    zone_id TEXT NOT NULL,
    community_set_id TEXT NOT NULL,
    community_id TEXT NOT NULL,
    member_ordinal INTEGER NOT NULL CHECK (member_ordinal >= 0),
    entity_id TEXT NOT NULL,
    PRIMARY KEY (zone_id, community_set_id, community_id, member_ordinal),
    UNIQUE (zone_id, community_set_id, community_id, entity_id)
) STRICT;

CREATE INDEX community_members_by_entity
    ON community_members(zone_id, community_set_id, entity_id);

CREATE TABLE reports (
    zone_id TEXT NOT NULL,
    id TEXT NOT NULL,
    community_id TEXT NOT NULL,
    period TEXT NOT NULL,
    title TEXT NOT NULL,
    summary TEXT NOT NULL,
    rank REAL NOT NULL,
    rating_explanation TEXT NOT NULL,
    full_content TEXT NOT NULL,
    full_content_json TEXT NOT NULL,
    model TEXT NOT NULL,
    prompt_hash BLOB NOT NULL,
    tokenizer TEXT NOT NULL,
    max_input_tokens INTEGER NOT NULL CHECK (max_input_tokens > 0),
    max_report_length INTEGER NOT NULL CHECK (max_report_length > 0),
    PRIMARY KEY (zone_id, id)
) STRICT;

CREATE INDEX reports_by_community
    ON reports(zone_id, community_id);

CREATE TABLE report_entities (
    zone_id TEXT NOT NULL,
    report_id TEXT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    entity_id TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    PRIMARY KEY (zone_id, report_id, ordinal),
    UNIQUE (zone_id, report_id, entity_id)
) STRICT;

CREATE TABLE report_relations (
    zone_id TEXT NOT NULL,
    report_id TEXT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    relation_id TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    PRIMARY KEY (zone_id, report_id, ordinal),
    UNIQUE (zone_id, report_id, relation_id)
) STRICT;

CREATE TABLE report_claim_evidence (
    zone_id TEXT NOT NULL,
    report_id TEXT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    claim_id TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    evidence_index INTEGER NOT NULL CHECK (evidence_index >= 0),
    PRIMARY KEY (zone_id, report_id, ordinal),
    UNIQUE (zone_id, report_id, claim_id, version, evidence_index)
) STRICT;

CREATE TABLE report_text_units (
    zone_id TEXT NOT NULL,
    report_id TEXT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    text_unit_id TEXT NOT NULL,
    PRIMARY KEY (zone_id, report_id, ordinal),
    UNIQUE (zone_id, report_id, text_unit_id)
) STRICT;

CREATE TABLE report_sets (
    zone_id TEXT NOT NULL,
    id TEXT NOT NULL,
    community_set_id TEXT NOT NULL,
    corpora_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (zone_id, id)
) STRICT;

CREATE INDEX report_sets_by_community_set
    ON report_sets(zone_id, community_set_id);

CREATE TABLE community_report_publications (
    zone_id TEXT NOT NULL,
    structure_id TEXT NOT NULL,
    epoch_id INTEGER NOT NULL CHECK (epoch_id > 0),
    report_set_id TEXT NOT NULL,
    vectors_ready INTEGER NOT NULL CHECK (vectors_ready IN (0, 1)),
    created_at TEXT NOT NULL CHECK (length(trim(created_at)) > 0),
    PRIMARY KEY (zone_id, structure_id),
    UNIQUE (zone_id, epoch_id)
) STRICT;

CREATE TABLE report_set_reports (
    zone_id TEXT NOT NULL,
    report_set_id TEXT NOT NULL,
    community_id TEXT NOT NULL,
    report_id TEXT NOT NULL,
    PRIMARY KEY (zone_id, report_set_id, community_id),
    UNIQUE (zone_id, report_set_id, report_id)
) STRICT;

INSERT INTO community_schema (id, version)
VALUES (1, 11);
