CREATE TABLE analysis_cache_schema (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    version INTEGER NOT NULL CHECK (version > 0)
) STRICT;

INSERT INTO analysis_cache_schema (id, version) VALUES (1, 3);

CREATE TABLE analysis_cache (
    cache_key TEXT PRIMARY KEY CHECK (length(trim(cache_key)) > 0),
    capability TEXT NOT NULL CHECK (length(trim(capability)) > 0),
    payload BLOB NOT NULL CHECK (length(payload) > 0),
    created_at TEXT NOT NULL CHECK (length(trim(created_at)) > 0)
) STRICT;
