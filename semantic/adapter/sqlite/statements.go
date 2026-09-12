package sqlite

import "github.com/memoria-space/meking/semantic/adapter/sqlite/internal/db"

type statements = db.Querier

func newStatements(executor db.DBTX) statements {
	return db.New(executor)
}
