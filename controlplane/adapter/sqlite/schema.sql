CREATE TABLE controlplane_schema (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    version INTEGER NOT NULL CHECK (version > 0)
) STRICT;

INSERT INTO controlplane_schema (id, version) VALUES (1, 4);

CREATE TABLE controlplane_policies (
    action TEXT PRIMARY KEY CHECK (action IN (
        'convert_document',
        'create_text_units',
        'extract_knowledge',
        'index_entity_vectors',
        'derive_community_structure',
        'publish_epoch',
        'generate_community_reports'
    )),
    mode TEXT NOT NULL CHECK (mode IN ('automatic', 'manual', 'suspended')),
    minimum_pending INTEGER NOT NULL CHECK (minimum_pending >= 0),
    maximum_wait_nanoseconds INTEGER NOT NULL CHECK (maximum_wait_nanoseconds >= 0),
    revision INTEGER NOT NULL CHECK (revision > 0),
    updated_at TEXT NOT NULL CHECK (length(trim(updated_at)) > 0),
    CHECK (
        (mode = 'automatic' AND minimum_pending > 0 AND maximum_wait_nanoseconds > 0)
        OR
        (mode IN ('manual', 'suspended') AND minimum_pending = 0 AND maximum_wait_nanoseconds = 0)
    )
) STRICT;
