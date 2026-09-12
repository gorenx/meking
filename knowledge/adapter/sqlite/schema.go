package sqlite

import _ "embed"

const schemaVersion = 23

// schema is the exact DDL consumed both by runtime initialization and sqlc
// generation. Keeping one embedded SQL file prevents the adapter and generated
// CRUD contract from drifting.
//
//go:embed sql/schema.sql
var schema string
