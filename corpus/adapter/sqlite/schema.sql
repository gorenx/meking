CREATE TABLE corpus_schema (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    version INTEGER NOT NULL CHECK (version > 0)
) STRICT;

INSERT INTO corpus_schema (id, version) VALUES (1, 30);

CREATE TABLE corpus_child_boundaries (
    zone_id TEXT NOT NULL,
    child_zone_id TEXT NOT NULL,
    source_corpora_id INTEGER NOT NULL CHECK (source_corpora_id > 0),
    PRIMARY KEY (zone_id, child_zone_id),
    CHECK (zone_id <> child_zone_id)
) STRICT;

CREATE TABLE corpus_text_vector_progress (
    zone_id TEXT NOT NULL,
    corpora_id INTEGER NOT NULL CHECK (corpora_id > 0),
    text_id TEXT NOT NULL,
    source_event_id TEXT NOT NULL,
    PRIMARY KEY (zone_id, corpora_id, text_id),
    UNIQUE (zone_id, source_event_id)
) STRICT;

CREATE TABLE documents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    media_type TEXT NOT NULL,
    size INTEGER NOT NULL CHECK (size > 0),
    digest TEXT NOT NULL CHECK (length(digest) = 64),
    location TEXT NOT NULL,
    UNIQUE (digest),
    UNIQUE (location)
) STRICT;

CREATE TABLE zone_documents (
    zone_id TEXT NOT NULL,
    document_id TEXT NOT NULL,
    PRIMARY KEY (zone_id, document_id)
) STRICT;

CREATE TABLE texts (
    id TEXT NOT NULL,
    source_document_id TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    format TEXT NOT NULL CHECK (format IN ('plain_text', 'markdown')),
    extraction_profile TEXT NOT NULL,
    normalization_profile TEXT NOT NULL,
    PRIMARY KEY (id)
) STRICT;

CREATE UNIQUE INDEX texts_by_document_and_profiles
    ON texts(source_document_id, extraction_profile, normalization_profile);

CREATE TABLE corpus_local_texts (
    zone_id TEXT NOT NULL,
    text_id TEXT NOT NULL,
    PRIMARY KEY (zone_id, text_id)
) STRICT;

CREATE TABLE text_warnings (
    text_id TEXT NOT NULL,
    warning_position INTEGER NOT NULL CHECK (warning_position >= 0),
    warning TEXT NOT NULL,
    PRIMARY KEY (text_id, warning_position)
) STRICT;

CREATE TABLE text_units (
    zone_id TEXT NOT NULL,
    id TEXT NOT NULL,
    text TEXT NOT NULL,
    PRIMARY KEY (zone_id, id)
) STRICT;

-- Ordered Agent messages belonging to the Session represented by zone_id.
CREATE TABLE messages (
    zone_id TEXT NOT NULL,
    id TEXT NOT NULL CHECK (length(trim(id)) > 0),
    role TEXT NOT NULL CHECK (length(trim(role)) > 0),
    position INTEGER NOT NULL CHECK (position >= 0),
    text_unit_id TEXT NOT NULL,
    PRIMARY KEY (zone_id, id),
    UNIQUE (zone_id, position)
) STRICT;

CREATE TABLE text_unit_spans (
    zone_id TEXT NOT NULL,
    corpora_id INTEGER NOT NULL CHECK (corpora_id > 0),
    text_id TEXT NOT NULL,
    text_unit_id TEXT NOT NULL,
    start_char INTEGER NOT NULL CHECK (start_char >= 0),
    end_char INTEGER NOT NULL CHECK (end_char > start_char),
    token_count INTEGER NOT NULL CHECK (token_count > 0),
    PRIMARY KEY (zone_id, corpora_id, text_id, start_char, end_char)
) STRICT;

CREATE INDEX text_unit_spans_by_set_and_text_unit
    ON text_unit_spans(zone_id, corpora_id, text_unit_id);

CREATE TABLE text_chunking_progress (
    zone_id TEXT NOT NULL,
    text_id TEXT NOT NULL,
    source_event_id TEXT NOT NULL,
    next_chunk_index INTEGER NOT NULL CHECK (next_chunk_index >= 0),
    completed INTEGER NOT NULL CHECK (completed IN (0, 1)),
    PRIMARY KEY (zone_id, text_id),
    UNIQUE (zone_id, source_event_id)
) STRICT;

CREATE TABLE corpus_state (
    zone_id TEXT PRIMARY KEY,
    current_corpora_id INTEGER NOT NULL CHECK (current_corpora_id >= 0),
    building_corpora_id INTEGER,
    building_state TEXT CHECK (building_state IN ('building', 'prepared')),
    CHECK (
        (building_corpora_id IS NULL AND building_state IS NULL)
        OR
        (building_corpora_id = current_corpora_id + 1 AND building_state IS NOT NULL)
    )
) STRICT;
