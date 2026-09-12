package sqlite

import _ "embed"

const schemaVersion = 11

// schema is the Community adapter's exact DDL and is also the sqlc schema source.
//
//go:embed schema.sql
var schema string
