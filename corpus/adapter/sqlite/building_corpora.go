package sqlite

import (
	"context"
	"database/sql"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/zone"
)

func (s *Store) LoadBuildingCorpora(ctx context.Context, id corpus.CorporaID) (corpus.Corpora, error) {
	if err := corpus.ValidateCorporaID(id); err != nil {
		return corpus.Corpora{}, err
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return corpus.Corpora{}, err
	}
	transaction, err := s.database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return corpus.Corpora{}, classifySQLite("begin Building Corpora read", err)
	}
	defer transaction.Rollback()
	statements := newStatements(transaction)
	state, err := corpusState(ctx, statements, string(zoneID))
	if err != nil {
		return corpus.Corpora{}, classifySQLite("read Corpus state for Building Corpora", err)
	}
	sequence := corporaIDSequence(id)
	building := state.BuildingCorporaID.Valid && state.BuildingCorporaID.Int64 == sequence &&
		(state.BuildingState.String == "building" || state.BuildingState.String == "prepared")
	if !corpusReadable(state, id) && !building {
		return corpus.Corpora{}, corpus.ErrCorporaNotFound
	}
	return loadCorporaContent(ctx, statements, string(zoneID), id)
}
