package assembly

import (
	"database/sql"
	"errors"
	"log/slog"
	"path/filepath"

	"github.com/memoria-space/meking/internal/sqlitepool"
	"github.com/memoria-space/meking/semantic"
	semanticsqlite "github.com/memoria-space/meking/semantic/adapter/sqlite"
)

const (
	entityVectorDatabaseFile   = "entity-vectors.sqlite"
	textUnitVectorDatabaseFile = "text-unit-vectors.sqlite"
	reportVectorDatabaseFile   = "report-vectors.sqlite"
)

func openVectorDatabases(
	root string,
	model string,
	logger *slog.Logger,
) (_ *vectorDatabases, resultErr error) {
	databases := &vectorDatabases{}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, databases.Close())
		}
	}()
	var err error
	databases.Entities, err = openVectorDatabase(
		filepath.Join(root, entityVectorDatabaseFile),
		model,
		logger,
	)
	if err != nil {
		return nil, err
	}
	databases.TextUnits, err = openVectorDatabase(
		filepath.Join(root, textUnitVectorDatabaseFile),
		model,
		logger,
	)
	if err != nil {
		return nil, err
	}
	databases.Reports, err = openVectorDatabase(
		filepath.Join(root, reportVectorDatabaseFile),
		model,
		logger,
	)
	if err != nil {
		return nil, err
	}
	return databases, nil
}

type vectorDatabase struct {
	semantic.NamespaceStore
	database *sql.DB
	store    *semanticsqlite.Store
}

type vectorDatabases struct {
	Entities  *vectorDatabase
	TextUnits *vectorDatabase
	Reports   *vectorDatabase
}

func openVectorDatabase(
	path string,
	model string,
	logger *slog.Logger,
) (_ *vectorDatabase, resultErr error) {
	database, err := sqlitepool.Open(path)
	if err != nil {
		return nil, err
	}
	opened := &vectorDatabase{database: database}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, opened.Close())
		}
	}()
	opened.store, err = semanticsqlite.NewStore(database, model, logger)
	if err != nil {
		return nil, err
	}
	opened.NamespaceStore = opened.store
	return opened, nil
}

type tokenCounter interface {
	Count(string) (int, error)
}

func (database *vectorDatabase) NewService(
	embedder semantic.Embedder,
	tokens tokenCounter,
	config semantic.GenerationConfig,
) (*semantic.Service, error) {
	if database == nil || database.store == nil {
		return nil, errors.New("create Semantic service: database is closed")
	}
	return semantic.NewService(embedder, tokens, database.store, config)
}

func (database *vectorDatabase) Close() error {
	if database == nil {
		return nil
	}
	pool := database.database
	database.NamespaceStore = nil
	database.store = nil
	database.database = nil
	if pool == nil {
		return nil
	}
	return pool.Close()
}

func (databases *vectorDatabases) Close() error {
	if databases == nil {
		return nil
	}
	reports := databases.Reports
	textUnits := databases.TextUnits
	entities := databases.Entities
	*databases = vectorDatabases{}
	var reportErr, textUnitErr, entityErr error
	if reports != nil {
		reportErr = reports.Close()
	}
	if textUnits != nil {
		textUnitErr = textUnits.Close()
	}
	if entities != nil {
		entityErr = entities.Close()
	}
	return errors.Join(reportErr, textUnitErr, entityErr)
}
