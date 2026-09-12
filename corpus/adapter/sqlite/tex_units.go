package sqlite

import (
	"context"
	"fmt"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/adapter/sqlite/internal/db"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/zone"
)

func (store *Store) LoadChunkedText(
	ctx context.Context,
	corporaID corpus.CorporaID,
	textID text.ID,
) (corpus.ChunkedText, bool, error) {
	if corpus.ValidateCorporaID(corporaID) != nil || text.ValidateID(textID) != nil {
		return corpus.ChunkedText{}, false, corpus.ErrInvalidCorpus
	}
	statements, zoneID, err := store.reader(ctx)
	if err != nil {
		return corpus.ChunkedText{}, false, err
	}
	state, err := statements.GetCorpusState(ctx, zoneID)
	if err != nil {
		return corpus.ChunkedText{}, false, classifySQLite("read Corpus state", err)
	}
	if !state.BuildingCorporaID.Valid ||
		state.BuildingCorporaID.Int64 != corporaIDSequence(corporaID) ||
		state.BuildingState.String != "building" {
		return corpus.ChunkedText{}, false, corpus.ErrCorporaPreparationConflict
	}
	rows, err := statements.ListCorporaTextUnitSpans(ctx, db.ListCorporaTextUnitSpansParams{
		ZoneID: zoneID, CorporaID: corporaIDSequence(corporaID), TextID: string(textID),
	})
	if err != nil {
		return corpus.ChunkedText{}, false, classifySQLite("read Building Corpus TextUnitSpans", err)
	}
	if len(rows) == 0 {
		return corpus.ChunkedText{}, false, nil
	}
	spans := make([]textunits.TextUnit, len(rows))
	for index, row := range rows {
		spans[index] = textunits.TextUnit{
			TextUnit: textunits.TextUnitBody{
				ID: textunits.TextUnitID(row.TextUnitID), Text: row.TextUnitText,
			},
			StartIndex: int(row.StartChar), EndIndex: int(row.EndChar), TokenCount: int(row.TokenCount),
		}
	}
	result, err := corpus.NewChunkedText(textID, spans)
	if err != nil {
		return corpus.ChunkedText{}, false, fmt.Errorf(
			"%w: restore Building Corpus TextUnitSpans: %v", corpus.ErrCorpusDataIntegrity, err,
		)
	}
	textValue, err := loadText(ctx, statements, zoneID, textID)
	if err != nil {
		return corpus.ChunkedText{}, false, err
	}
	for _, span := range result.TextUnits {
		if err := corpus.ValidateTextUnitContent(textValue, span); err != nil {
			return corpus.ChunkedText{}, false, fmt.Errorf("%w: %v", corpus.ErrCorpusDataIntegrity, err)
		}
	}
	return result, true, nil
}

func (store *Store) SaveTextUnits(
	ctx context.Context,
	corporaID corpus.CorporaID,
	value corpus.ChunkedText,
) error {
	if err := corpus.ValidateCorporaID(corporaID); err != nil {
		return err
	}
	prepared, err := corpus.NewChunkedText(value.TextID, value.TextUnits)
	if err != nil {
		return err
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return err
	}
	zoneIDValue := string(zoneID)
	return store.write(ctx, zoneIDValue, func(statements statements) error {
		state, err := statements.GetCorpusState(ctx, zoneIDValue)
		if err != nil {
			return classifySQLite("read Corpus state", err)
		}
		if !state.BuildingCorporaID.Valid ||
			state.BuildingCorporaID.Int64 != corporaIDSequence(corporaID) ||
			state.BuildingState.String != "building" {
			return corpus.ErrCorporaPreparationConflict
		}
		textValue, err := loadText(ctx, statements, zoneIDValue, prepared.TextID)
		if err != nil {
			return err
		}
		for _, span := range prepared.TextUnits {
			if err := corpus.ValidateTextUnitContent(textValue, span); err != nil {
				return err
			}
		}
		for _, span := range prepared.TextUnits {
			if err := saveTextUnit(ctx, statements, zoneIDValue, span.TextUnit); err != nil {
				return err
			}
			if err := statements.AddTextUnitSpan(ctx, db.AddTextUnitSpanParams{
				ZoneID: zoneIDValue, CorporaID: corporaIDSequence(corporaID), TextID: string(prepared.TextID),
				TextUnitID: string(span.TextUnit.ID), StartChar: int64(span.StartIndex),
				EndChar: int64(span.EndIndex), TokenCount: int64(span.TokenCount),
			}); err != nil {
				return classifySQLite("add Building Corpus TextUnitSpan", err)
			}
		}
		return nil
	})
}
