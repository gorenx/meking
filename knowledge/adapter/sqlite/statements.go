package sqlite

import "github.com/memoria-space/meking/knowledge/adapter/sqlite/internal/db"

type statements struct {
	db.Querier
	zoneID string
}

func newStatements(executor db.DBTX, zoneID string) statements {
	return statements{Querier: db.New(executor), zoneID: zoneID}
}
