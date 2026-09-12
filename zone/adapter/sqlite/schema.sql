CREATE TABLE zone_schema (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    version INTEGER NOT NULL CHECK (version > 0)
) STRICT;

INSERT INTO zone_schema (id, version) VALUES (1, 1);

CREATE TABLE zones (
    id TEXT PRIMARY KEY,
    parent_zone_id TEXT,
    -- Only an MCP-managed Root stores the external User identity; Child rows inherit it through parent_zone_id.
    user_id TEXT,
    role TEXT NOT NULL CHECK (role IN ('root', 'child')),
    created_at TEXT NOT NULL,
    CHECK (user_id IS NULL OR (length(user_id) BETWEEN 1 AND 256 AND user_id = trim(user_id) AND instr(user_id, '/') = 0)),
    CHECK (
        (role = 'root' AND parent_zone_id IS NULL)
        OR
        (role = 'child' AND parent_zone_id IS NOT NULL AND parent_zone_id <> id AND user_id IS NULL)
    )
) STRICT;

CREATE INDEX zones_by_parent_and_created
    ON zones(parent_zone_id, created_at, id);

-- One User resolves to exactly one Root while manually created Roots may keep user_id NULL.
CREATE UNIQUE INDEX zone_root_by_user
    ON zones(user_id)
    WHERE user_id IS NOT NULL;
