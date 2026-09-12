package sqlite

import _ "embed"

const schemaVersion = 9

//go:embed schema.sql
var schema string
