package sqlite

import (
	db2 "github.com/memoria-space/meking/corpus/adapter/sqlite/internal/db"
)

// statements is the adapter-local name for the sqlc-generated Corpus CRUD
// contract. A value is bound to one pool connection or transaction and never
// crosses the adapter.
type statements = db2.Querier

func newStatements(executor db2.DBTX) statements {
	return db2.New(executor)
}
