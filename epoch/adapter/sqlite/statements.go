package sqlite

import "github.com/memoria-space/meking/epoch/adapter/sqlite/internal/db"

// statements is the adapter-local name for the sqlc-generated Epoch CRUD
// contract. It is bound to one connection or transaction for its lifetime.
type statements struct {
	db.Querier
	zoneID string
}

func newStatements(executor db.DBTX, zoneID string) statements {
	return statements{Querier: db.New(executor), zoneID: zoneID}
}

func newGlobalStatements(executor db.DBTX) statements {
	return statements{Querier: db.New(executor)}
}
