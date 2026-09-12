package sqlite

import _ "embed"

const schemaVersion = 30

// schema is the Corpus adapter's exact DDL and is also the sqlc schema source.
//
//go:embed schema.sql
var schema string
