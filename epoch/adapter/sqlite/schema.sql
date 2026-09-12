CREATE TABLE epoch_schema (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    version INTEGER NOT NULL CHECK (version > 0)
) STRICT;

INSERT INTO epoch_schema (id, version) VALUES (1, 11);

CREATE TABLE epochs (
    zone_id TEXT NOT NULL,
    id INTEGER NOT NULL CHECK (id > 0),
	knowledge_digest TEXT NOT NULL CHECK (length(knowledge_digest) = 64),
	corpora_id TEXT NOT NULL CHECK (length(trim(corpora_id)) > 0),
	structure_id TEXT NOT NULL CHECK (length(structure_id) = 36),
    published_at TEXT NOT NULL CHECK (length(trim(published_at)) > 0),
    PRIMARY KEY (zone_id, id)
) STRICT;

CREATE TABLE epoch_entity_versions (
    zone_id TEXT NOT NULL,
    epoch_id INTEGER NOT NULL CHECK (epoch_id > 0),
    entity_id TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    PRIMARY KEY (zone_id, epoch_id, entity_id)
) STRICT;

CREATE TABLE epoch_relation_versions (
    zone_id TEXT NOT NULL,
    epoch_id INTEGER NOT NULL CHECK (epoch_id > 0),
    relation_id TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    PRIMARY KEY (zone_id, epoch_id, relation_id)
) STRICT;

CREATE TABLE epoch_claim_versions (
    zone_id TEXT NOT NULL,
    epoch_id INTEGER NOT NULL CHECK (epoch_id > 0),
    claim_id TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    PRIMARY KEY (zone_id, epoch_id, claim_id)
) STRICT;

CREATE UNIQUE INDEX epochs_by_structure
    ON epochs(zone_id, structure_id);

CREATE TABLE current_epoch (
    zone_id TEXT PRIMARY KEY,
    epoch_id INTEGER NOT NULL CHECK (epoch_id > 0),
    UNIQUE (zone_id, epoch_id)
) STRICT;
