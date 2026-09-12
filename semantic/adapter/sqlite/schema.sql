CREATE TABLE semantic_schema (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    version INTEGER NOT NULL CHECK (version > 0)
) STRICT;

INSERT INTO semantic_schema (id, version) VALUES (1, 9);

CREATE TABLE vector_database (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    model TEXT NOT NULL CHECK (length(trim(model)) > 0),
    dimension INTEGER NOT NULL CHECK (dimension >= 0)
);

CREATE TABLE vectors (
    id INTEGER PRIMARY KEY,
    vector_id TEXT NOT NULL CHECK (
        length(trim(vector_id)) > 0 AND trim(vector_id) = vector_id
    ),
    embedding TEXT NOT NULL CHECK (length(embedding) > 0),
    UNIQUE (vector_id)
);

CREATE TABLE namespace_config (
    id INTEGER PRIMARY KEY,
    zone_id TEXT NOT NULL CHECK (
        length(trim(zone_id)) > 0 AND trim(zone_id) = zone_id
    ),
    name TEXT NOT NULL CHECK (
        length(trim(name)) > 0 AND trim(name) = name
    ),
    UNIQUE (zone_id, name),
    UNIQUE (zone_id, id)
);

CREATE TABLE namespace_vectors (
    id INTEGER PRIMARY KEY,
    zone_id TEXT NOT NULL CHECK (
        length(trim(zone_id)) > 0 AND trim(zone_id) = zone_id
    ),
    namespace_row_id INTEGER NOT NULL CHECK (namespace_row_id > 0),
    vector_row_id INTEGER NOT NULL CHECK (vector_row_id > 0),
    UNIQUE (zone_id, namespace_row_id, vector_row_id),
    FOREIGN KEY (zone_id, namespace_row_id) REFERENCES namespace_config(zone_id, id),
    FOREIGN KEY (vector_row_id) REFERENCES vectors(id)
);
