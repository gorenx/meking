CREATE TABLE activation_schema (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    version INTEGER NOT NULL CHECK (version = 1)
);
INSERT INTO activation_schema (id, version) VALUES (1, 1);

CREATE TABLE activation_protocols (
    digest TEXT PRIMARY KEY,
    version TEXT NOT NULL,
    language TEXT NOT NULL,
    prompt TEXT NOT NULL,
    input_schema TEXT NOT NULL,
    output_schema TEXT NOT NULL
);
CREATE TABLE activation_models (
    model_id TEXT PRIMARY KEY,
    formula TEXT NOT NULL,
    parameters TEXT NOT NULL
);
CREATE TABLE activation_observations (
    zone_id TEXT NOT NULL,
    observation_id TEXT NOT NULL,
    subject_kind TEXT NOT NULL CHECK (subject_kind IN ('entity', 'relation', 'claim')),
    subject_id TEXT NOT NULL,
    version TEXT NOT NULL,
    occurred_at TEXT NOT NULL,
    protocol_digest TEXT NOT NULL REFERENCES activation_protocols(digest),
    model_id TEXT NOT NULL REFERENCES activation_models(model_id),
    applied_sequence INTEGER CHECK (applied_sequence > 0),
    input TEXT NOT NULL,
    recorded_at TEXT NOT NULL,
    PRIMARY KEY (zone_id, observation_id),
    UNIQUE (zone_id, subject_kind, subject_id, applied_sequence)
);
CREATE TABLE activation_states (
    zone_id TEXT NOT NULL,
    subject_kind TEXT NOT NULL CHECK (subject_kind IN ('entity', 'relation', 'claim')),
    subject_id TEXT NOT NULL,
    stability REAL NOT NULL CHECK (stability >= 0.001),
    difficulty REAL NOT NULL CHECK (difficulty >= 1 AND difficulty <= 10),
    last_review TEXT NOT NULL,
    applied_sequence INTEGER NOT NULL CHECK (applied_sequence > 0),
    model_id TEXT NOT NULL REFERENCES activation_models(model_id),
    PRIMARY KEY (zone_id, subject_kind, subject_id)
);
