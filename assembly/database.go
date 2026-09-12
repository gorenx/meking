package assembly

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/memoria-space/meking/internal/sqlitepool"
	projectlocal "github.com/memoria-space/meking/project/adapter/local"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
)

const projectDatabaseFile = "project.sqlite"

type databaseResources struct {
	root         string
	database     *sql.DB
	transactions *transactionsqlite.Tx
	lock         *projectlocal.ProjectServiceLock
}

func openDatabase(ctx context.Context, root string) (_ databaseResources, resultErr error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return databaseResources{}, fmt.Errorf("resolve Project root: %w", err)
	}
	absolute, err = filepath.EvalSymlinks(filepath.Clean(absolute))
	if err != nil {
		return databaseResources{}, fmt.Errorf("resolve Project root links: %w", err)
	}
	lock, err := projectlocal.AcquireProjectServiceLock(absolute)
	if err != nil {
		return databaseResources{}, err
	}
	resources := databaseResources{root: absolute, lock: lock}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, resources.close())
		}
	}()
	resources.database, err = sqlitepool.Open(filepath.Join(absolute, projectDatabaseFile))
	if err != nil {
		return databaseResources{}, err
	}
	resources.transactions, err = transactionsqlite.New(resources.database)
	if err != nil {
		return databaseResources{}, err
	}
	return resources, nil
}

func (resources *databaseResources) close() error {
	if resources == nil {
		return nil
	}
	database := resources.database
	lock := resources.lock
	resources.database = nil
	resources.transactions = nil
	resources.lock = nil
	var databaseErr, lockErr error
	if database != nil {
		databaseErr = database.Close()
	}
	if lock != nil {
		lockErr = lock.Close()
	}
	return errors.Join(databaseErr, lockErr)
}
