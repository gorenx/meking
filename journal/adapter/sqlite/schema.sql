CREATE TABLE journal_schema (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    version INTEGER NOT NULL CHECK (version > 0)
) STRICT;

INSERT INTO journal_schema (id, version) VALUES (1, 5);

CREATE TABLE journal_events (
    event_id TEXT PRIMARY KEY,
    zone_id TEXT NOT NULL,
    stream_id TEXT NOT NULL,
    stream_sequence INTEGER NOT NULL CHECK (stream_sequence > 0),
    event_type TEXT NOT NULL,
    -- Zero-based sequence within one Zone and EventType.
    event_sequence INTEGER NOT NULL CHECK (event_sequence >= 0),
    schema_version INTEGER NOT NULL CHECK (schema_version > 0),
    occurred_at TEXT NOT NULL,
    correlation_id TEXT NOT NULL,
    causation_id TEXT NOT NULL,
    body TEXT NOT NULL CHECK (json_valid(body)),
    UNIQUE (zone_id, stream_id, stream_sequence),
    UNIQUE (zone_id, event_type, event_sequence)
) STRICT;

CREATE INDEX journal_events_by_stream
    ON journal_events(zone_id, stream_id, stream_sequence);

CREATE INDEX journal_events_by_zone_key_sequence
    ON journal_events(zone_id, event_type, schema_version, event_sequence);

CREATE TABLE journal_consumer_positions (
    zone_id TEXT NOT NULL,
    consumer_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    -- Next EventType sequence to consume.
    position INTEGER NOT NULL CHECK (position >= 0),
    PRIMARY KEY (zone_id, consumer_id, event_type)
) STRICT;
