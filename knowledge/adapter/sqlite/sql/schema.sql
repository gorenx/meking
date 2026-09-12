CREATE TABLE knowledge_schema (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    -- Version of the complete Knowledge schema installed in this database.
    version INTEGER NOT NULL CHECK (version > 0)
) STRICT;

INSERT INTO knowledge_schema (id, version) VALUES (1, 23);

-- Stable Entity identity. Description and aliases belong to immutable Versions.
CREATE TABLE entity_identities (
    -- Knowledge facts are isolated by the Zone that owns them.
    zone_id TEXT NOT NULL,
    -- Permanent identity reused by every Version of the same Entity.
    entity_id TEXT NOT NULL CHECK (length(trim(entity_id)) > 0),
    -- Canonical title participating in Entity identity.
    title TEXT NOT NULL CHECK (length(trim(title)) > 0),
    -- Canonical type participating in Entity identity.
    entity_type TEXT NOT NULL CHECK (length(trim(entity_type)) > 0),
    PRIMARY KEY (zone_id, entity_id),
    UNIQUE (zone_id, title, entity_type)
) STRICT;

-- Stable directed Relation identity. Description belongs to immutable Versions.
CREATE TABLE relation_identities (
    zone_id TEXT NOT NULL,
    -- Permanent identity reused by every Version of the same Relation.
    relation_id TEXT NOT NULL CHECK (length(trim(relation_id)) > 0),
    -- Source and target are ordered; reversing them identifies another Relation.
    source_entity_id TEXT NOT NULL,
    target_entity_id TEXT NOT NULL,
    -- Relation type is part of identity, so the same endpoints may have several Relations.
    relation_type TEXT NOT NULL CHECK (length(trim(relation_type)) > 0),
    PRIMARY KEY (zone_id, relation_id),
    UNIQUE (zone_id, source_entity_id, target_entity_id, relation_type),
    FOREIGN KEY (zone_id, source_entity_id)
        REFERENCES entity_identities(zone_id, entity_id),
    FOREIGN KEY (zone_id, target_entity_id)
        REFERENCES entity_identities(zone_id, entity_id)
) STRICT;

CREATE INDEX relation_identities_by_target
    ON relation_identities(zone_id, target_entity_id);

-- Stable Claim identity. A TextUnit may produce several independently identified Claims.
CREATE TABLE claim_identities (
    zone_id TEXT NOT NULL,
    -- Permanent identity reused by every Version of the same Claim.
    claim_id TEXT NOT NULL CHECK (length(trim(claim_id)) > 0),
    -- Discriminator for the Entity or Relation identified by subject_id.
    subject_kind TEXT NOT NULL CHECK (subject_kind IN ('entity', 'relation')),
    -- Stable Knowledge identity of the Claim subject; validated against subject_kind by the adapter.
    subject_id TEXT NOT NULL CHECK (length(trim(subject_id)) > 0),
    -- Claim type is part of identity; Description is versioned content.
    claim_type TEXT NOT NULL CHECK (length(trim(claim_type)) > 0),
    PRIMARY KEY (zone_id, claim_id),
    UNIQUE (zone_id, subject_kind, subject_id, claim_type)
) STRICT;

CREATE INDEX claim_identities_by_subject
    ON claim_identities(zone_id, subject_kind, subject_id);

-- Immutable formal and conflict content for Entity, Relation, and Claim identities.
CREATE TABLE knowledge_versions (
    zone_id TEXT NOT NULL,
    -- Selects the identity table and the formal-content rules used by the adapter.
    knowledge_kind TEXT NOT NULL CHECK (knowledge_kind IN ('entity', 'relation', 'claim')),
    -- Stable EntityID, RelationID, or ClaimID selected by knowledge_kind.
    knowledge_id TEXT NOT NULL CHECK (length(trim(knowledge_id)) > 0),
    -- Formal base Version N. Current is the greatest row whose candidate_number is zero.
    version INTEGER NOT NULL CHECK (version BETWEEN 1 AND 9223372036854775807),
    -- Zero identifies formal N; a positive value identifies conflict Version N.x.
    candidate_number INTEGER NOT NULL CHECK (candidate_number >= 0),
    -- Tombstones are formal Versions created only through the explicit deletion use case.
    deleted INTEGER NOT NULL CHECK (deleted IN (0, 1)),
    -- SHA-256 of the complete canonical formal content, used for exact idempotency comparison.
    content_hash BLOB NOT NULL CHECK (length(content_hash) = 32),
    -- Canonical versioned Description; identity fields remain in their identity table.
    description TEXT NOT NULL CHECK (length(trim(description)) > 0),
    -- Source that first created this formal or conflict Version.
    created_by_source_id TEXT NOT NULL,
    -- Resolution Source for N.x; formal Versions never carry conflict resolution state.
    resolved_by_source_id TEXT,
    CHECK (candidate_number = 0 OR deleted = 0),
    CHECK (candidate_number > 0 OR resolved_by_source_id IS NULL),
    PRIMARY KEY (zone_id, knowledge_kind, knowledge_id, version, candidate_number),
    FOREIGN KEY (zone_id, created_by_source_id)
        REFERENCES knowledge_sources(zone_id, source_id),
    FOREIGN KEY (zone_id, resolved_by_source_id)
        REFERENCES knowledge_sources(zone_id, source_id)
) STRICT;

-- Aliases belong to one immutable formal Version N or conflict Version N.x.
CREATE TABLE entity_version_aliases (
    zone_id TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    candidate_number INTEGER NOT NULL CHECK (candidate_number >= 0),
    alias TEXT NOT NULL CHECK (length(trim(alias)) > 0),
    PRIMARY KEY (zone_id, entity_id, version, candidate_number, alias),
    FOREIGN KEY (zone_id, entity_id)
        REFERENCES entity_identities(zone_id, entity_id)
) STRICT;

-- Immutable fact identifying one concrete Knowledge input or review operation.
CREATE TABLE knowledge_sources (
    zone_id TEXT NOT NULL,
    -- Caller-assigned operation identity scoped by Zone; its owning use case defines reuse semantics.
    source_id TEXT NOT NULL CHECK (length(trim(source_id)) > 0),
    -- Business origin that determines how the Source entered Knowledge.
    source_kind TEXT NOT NULL CHECK (
        source_kind IN ('agent', 'extraction', 'resolution', 'deletion', 'child_zone', 'migration')
    ),
    -- Stable command, event, task, or merge identity in the producing boundary.
    producer_id TEXT NOT NULL CHECK (length(trim(producer_id)) > 0),
    PRIMARY KEY (zone_id, source_id)
) STRICT;

-- One Source's support and non-formal attributes for formal N or conflict N.x.
CREATE TABLE knowledge_source_metadata (
    zone_id TEXT NOT NULL,
    source_id TEXT NOT NULL,
    knowledge_kind TEXT NOT NULL CHECK (knowledge_kind IN ('entity', 'relation', 'claim')),
    knowledge_id TEXT NOT NULL CHECK (length(trim(knowledge_id)) > 0),
    version INTEGER NOT NULL CHECK (version > 0),
    candidate_number INTEGER NOT NULL CHECK (candidate_number >= 0),
    -- Canonical JSON object selected by knowledge_kind: Entity frequency, Relation weight,
    -- or Claim records with subject/object text, status, dates, and source text.
    metadata_json TEXT NOT NULL CHECK (
        json_valid(metadata_json) AND json_type(metadata_json) = 'object'
    ),
    PRIMARY KEY (
        zone_id,
        source_id,
        knowledge_kind,
        knowledge_id,
        version,
        candidate_number
    ),
    FOREIGN KEY (zone_id, source_id)
        REFERENCES knowledge_sources(zone_id, source_id),
    FOREIGN KEY (zone_id, knowledge_kind, knowledge_id, version, candidate_number)
        REFERENCES knowledge_versions(
            zone_id,
            knowledge_kind,
            knowledge_id,
            version,
            candidate_number
        )
) STRICT;

-- Evidence belongs to the Source as a whole. Different evidence requires a different Source.
CREATE TABLE knowledge_evidence (
    zone_id TEXT NOT NULL,
    source_id TEXT NOT NULL,
    -- Canonical sorted JSON array of exact Corpora or Message TextUnit sources.
    evidence_json TEXT NOT NULL CHECK (
        json_valid(evidence_json) AND json_type(evidence_json) = 'array'
    ),
    PRIMARY KEY (zone_id, source_id),
    FOREIGN KEY (zone_id, source_id)
        REFERENCES knowledge_sources(zone_id, source_id)
) STRICT;

-- Incremental extraction checkpoint for one immutable ordered Corpora collection.
CREATE TABLE knowledge_extraction_progress (
    zone_id TEXT NOT NULL,
    -- Identifies the immutable ordered TextUnit collection being extracted.
    corpora_id TEXT NOT NULL CHECK (length(trim(corpora_id)) > 0),
    -- Last TextUnit atomically submitted. No row means extraction has not started.
    last_text_unit_id TEXT NOT NULL CHECK (length(trim(last_text_unit_id)) > 0),
    PRIMARY KEY (zone_id, corpora_id)
) STRICT;

-- Parent content assigned to a Child Candidate and resolved under Child ownership.
CREATE TABLE zone_knowledge_conflicts (
    child_zone_id TEXT NOT NULL,
    knowledge_kind TEXT NOT NULL CHECK (knowledge_kind IN ('entity', 'relation', 'claim')),
    child_knowledge_id TEXT NOT NULL CHECK (length(trim(child_knowledge_id)) > 0),
    -- Child formal Version N against which the parent content became N.x.
    child_base_version INTEGER NOT NULL CHECK (child_base_version > 0),
    child_candidate_number INTEGER NOT NULL CHECK (child_candidate_number > 0),
    parent_zone_id TEXT NOT NULL,
    parent_knowledge_id TEXT NOT NULL CHECK (length(trim(parent_knowledge_id)) > 0),
    -- Exact parent Version whose content was assigned; later parent Versions trigger another review.
    parent_version INTEGER NOT NULL CHECK (parent_version > 0),
    -- Child Resolution Source; NULL means the assigned Candidate is still unresolved.
    resolved_by_source_id TEXT,
    -- Becomes true only after the resolved Child result has been applied back to the parent.
    parent_reconciled INTEGER NOT NULL DEFAULT 0 CHECK (parent_reconciled IN (0, 1)),
    CHECK (parent_reconciled = 0 OR resolved_by_source_id IS NOT NULL),
    PRIMARY KEY (
        child_zone_id,
        knowledge_kind,
        child_knowledge_id,
        child_base_version,
        child_candidate_number,
        parent_zone_id,
        parent_knowledge_id,
        parent_version
    ),
    FOREIGN KEY (
        child_zone_id,
        knowledge_kind,
        child_knowledge_id,
        child_base_version,
        child_candidate_number
    ) REFERENCES knowledge_versions(
        zone_id,
        knowledge_kind,
        knowledge_id,
        version,
        candidate_number
    ),
    -- parent_version always identifies formal N; the adapter validates candidate_number = 0.
    FOREIGN KEY (child_zone_id, resolved_by_source_id)
        REFERENCES knowledge_sources(zone_id, source_id)
) STRICT;

-- One unresolved parent assignment per Child identity and Base Version.
CREATE UNIQUE INDEX active_zone_knowledge_conflict
    ON zone_knowledge_conflicts(
        child_zone_id,
        knowledge_kind,
        child_knowledge_id,
        child_base_version
    )
    WHERE resolved_by_source_id IS NULL;
