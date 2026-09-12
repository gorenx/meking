package analysis

import _ "embed"

const sqliteCacheSchemaVersion = 3

//go:embed sqlite_cache_schema.sql
var sqliteCacheSchema string
