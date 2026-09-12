package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/adapter/sqlite/internal/db"
	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/zone"
)

const textUnitQueryBatchSize = 500

func (s *Store) Load(ctx context.Context, id corpus.CorporaID) (corpus.Corpora, error) {
	if err := corpus.ValidateCorporaID(id); err != nil {
		return corpus.Corpora{}, err
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return corpus.Corpora{}, err
	}
	transaction, err := s.database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return corpus.Corpora{}, classifySQLite("begin Corpus read", err)
	}
	defer transaction.Rollback()
	statements := newStatements(transaction)
	state, err := corpusState(ctx, statements, string(zoneID))
	if err != nil {
		return corpus.Corpora{}, classifySQLite("read Corpus state", err)
	}
	if !corpusReadable(state, id) {
		return corpus.Corpora{}, corpus.ErrCorporaNotFound
	}
	return loadCorporaContent(ctx, statements, string(zoneID), id)
}

func (s *Store) Current(ctx context.Context) (corpus.Corpora, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return corpus.Corpora{}, err
	}
	transaction, err := s.database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return corpus.Corpora{}, classifySQLite("begin current Corpus read", err)
	}
	defer transaction.Rollback()
	statements := newStatements(transaction)
	id, err := statements.GetCurrentCorporaID(ctx, string(zoneID))
	if errors.Is(err, sql.ErrNoRows) {
		return corpus.Corpora{}, corpus.ErrCorporaNotFound
	}
	if err != nil {
		return corpus.Corpora{}, classifySQLite("read current Corpus ID", err)
	}
	if id <= 0 {
		return corpus.Corpora{}, corpus.ErrCorporaNotFound
	}
	corporaID, err := corpus.NewCorporaID(id)
	if err != nil {
		return corpus.Corpora{}, corpus.ErrCorpusDataIntegrity
	}
	return loadCorporaContent(ctx, statements, string(zoneID), corporaID)
}

func (s *Store) CurrentID(ctx context.Context) (corpus.CorporaID, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return "", err
	}
	id, err := newStatements(s.database).GetCurrentCorporaID(ctx, string(zoneID))
	if errors.Is(err, sql.ErrNoRows) {
		return "", corpus.ErrCorporaNotFound
	}
	if err != nil {
		return "", classifySQLite("read current Corpus ID", err)
	}
	if id <= 0 {
		return "", corpus.ErrCorporaNotFound
	}
	corporaID, err := corpus.NewCorporaID(id)
	if err != nil {
		return "", corpus.ErrCorpusDataIntegrity
	}
	return corporaID, nil
}

func (s *Store) ExistingTextUnits(ctx context.Context, ids []textunits.TextUnitID) (map[textunits.TextUnitID]struct{}, error) {
	units, err := s.TextUnits(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make(map[textunits.TextUnitID]struct{}, len(units))
	for _, unit := range units {
		result[unit.ID] = struct{}{}
	}
	return result, nil
}

func (s *Store) TextUnits(ctx context.Context, ids []textunits.TextUnitID) ([]textunits.TextUnitBody, error) {
	normalized := normalizeTextUnitIDs(ids)
	if len(normalized) == 0 {
		return nil, nil
	}
	statements, zoneID, err := s.reader(ctx)
	if err != nil {
		return nil, err
	}
	state, err := corpusState(ctx, statements, zoneID)
	if err != nil {
		return nil, classifySQLite("read Corpus state for TextUnit lookup", err)
	}
	exclusiveCorporaID := state.CurrentCorporaID + 1
	if state.BuildingCorporaID.Valid {
		exclusiveCorporaID = state.BuildingCorporaID.Int64
		if state.BuildingState.String == "prepared" {
			exclusiveCorporaID++
		}
	}
	result := make([]textunits.TextUnitBody, 0, len(normalized))
	for start := 0; start < len(normalized); start += textUnitQueryBatchSize {
		end := min(start+textUnitQueryBatchSize, len(normalized))
		rows, err := statements.ListTextUnitsBeforeCorpora(ctx, db.ListTextUnitsBeforeCorporaParams{
			ZoneID:             zoneID,
			ExclusiveCorporaID: exclusiveCorporaID,
			TextUnitIds:        textUnitIDStrings(normalized[start:end]),
		})
		if err != nil {
			return nil, classifySQLite("list TextUnits before Corpus", err)
		}
		for _, row := range rows {
			unit := textunits.TextUnitBody{ID: textunits.TextUnitID(row.ID), Text: row.Text}
			if err := textunits.ValidateTextUnitBody(unit); err != nil {
				return nil, fmt.Errorf("%w: %v", corpus.ErrCorpusDataIntegrity, err)
			}
			result = append(result, unit)
		}
	}
	return result, nil
}

func (s *Store) TextUnitLocations(
	ctx context.Context,
	setID corpus.CorporaID,
	ids []textunits.TextUnitID,
) ([]corpus.TextUnitLocation, error) {
	normalized := normalizeTextUnitIDs(ids)
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return nil, err
	}
	transaction, err := s.database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, classifySQLite("begin TextUnit location read", err)
	}
	defer transaction.Rollback()
	statements := newStatements(transaction)
	state, err := corpusState(ctx, statements, string(zoneID))
	if err != nil {
		return nil, classifySQLite("read Corpus state", err)
	}
	if !corpusReadable(state, setID) {
		return nil, corpus.ErrCorporaNotFound
	}
	result := make([]corpus.TextUnitLocation, 0, len(normalized))
	for start := 0; start < len(normalized); start += textUnitQueryBatchSize {
		end := min(start+textUnitQueryBatchSize, len(normalized))
		rows, err := statements.ListTextUnitLocations(ctx,
			db.ListTextUnitLocationsParams{
				ZoneID:      string(zoneID),
				CorporaID:   corporaIDSequence(setID),
				TextUnitIds: textUnitIDStrings(normalized[start:end]),
			})
		if err != nil {
			return nil, classifySQLite("list TextUnit locations", err)
		}
		for _, row := range rows {
			textID := text.ID(row.TextID)
			if err := text.ValidateBody(row.TextBody); err != nil {
				return nil, fmt.Errorf("%w: Text %q body is invalid", corpus.ErrCorpusDataIntegrity, textID)
			}
			span := textunits.TextUnit{
				TextUnit: textunits.TextUnitBody{
					ID:   textunits.TextUnitID(row.TextUnitID),
					Text: row.TextUnitText,
				},
				StartIndex: int(row.StartChar),
				EndIndex:   int(row.EndChar),
				TokenCount: int(row.TokenCount),
			}
			if err = corpus.ValidateTextUnitContent(text.Text{ID: textID, Body: row.TextBody}, span); err != nil {
				return nil, fmt.Errorf("%w: %v", corpus.ErrCorpusDataIntegrity, err)
			}
			location := corpus.TextUnitLocation{
				CorporaID:        setID,
				TextID:           textID,
				TextTitle:        row.Title,
				DocumentID:       document.ID(row.SourceDocumentID),
				DocumentLocation: document.Location(row.DocumentLocation),
				TextUnit:         span,
			}
			if err := corpus.ValidateTextUnitLocation(location); err != nil {
				return nil, fmt.Errorf("%w: %v", corpus.ErrCorpusDataIntegrity, err)
			}
			result = append(result, location)
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].TextID != result[right].TextID {
			return result[left].TextID < result[right].TextID
		}
		if result[left].TextUnit.StartIndex != result[right].TextUnit.StartIndex {
			return result[left].TextUnit.StartIndex < result[right].TextUnit.StartIndex
		}
		return result[left].TextUnit.EndIndex < result[right].TextUnit.EndIndex
	})
	return result, nil
}

func loadCorporaContent(
	ctx context.Context,
	statements statements,
	zoneID string,
	id corpus.CorporaID,
) (corpus.Corpora, error) {
	rows, err := statements.ListTextUnitSpans(ctx, db.ListTextUnitSpansParams{
		ZoneID: zoneID, CorporaID: corporaIDSequence(id),
	})
	if err != nil {
		return corpus.Corpora{}, classifySQLite("list TextUnitSpans", err)
	}
	if len(rows) == 0 {
		return corpus.Corpora{}, corpus.ErrCorporaNotFound
	}
	chunkedTexts := make([]corpus.ChunkedText, 0)
	textCache := make(map[text.ID]text.Text)
	unitCount := 0
	for _, row := range rows {
		textID := text.ID(row.TextID)
		value, found := textCache[textID]
		if !found {
			value, err = loadText(ctx, statements, zoneID, textID)
			if err != nil {
				return corpus.Corpora{}, fmt.Errorf("%w: load Text %q: %v", corpus.ErrCorpusDataIntegrity, textID, err)
			}
			textCache[textID] = value
		}
		span := textunits.TextUnit{
			TextUnit:   textunits.TextUnitBody{ID: textunits.TextUnitID(row.TextUnitID), Text: row.TextUnitText},
			StartIndex: int(row.StartChar), EndIndex: int(row.EndChar),
			TokenCount: int(row.TokenCount),
		}
		if err := corpus.ValidateTextUnitContent(value, span); err != nil {
			return corpus.Corpora{}, fmt.Errorf("%w: %v", corpus.ErrCorpusDataIntegrity, err)
		}
		if len(chunkedTexts) == 0 || chunkedTexts[len(chunkedTexts)-1].TextID != textID {
			chunkedTexts = append(chunkedTexts, corpus.ChunkedText{TextID: textID})
		}
		last := len(chunkedTexts) - 1
		chunkedTexts[last].TextUnits = append(chunkedTexts[last].TextUnits, span)
		unitCount++
	}
	storedSpans, err := statements.CountTextUnitSpans(ctx, db.CountTextUnitSpansParams{
		ZoneID: zoneID, CorporaID: corporaIDSequence(id),
	})
	if err != nil {
		return corpus.Corpora{}, classifySQLite("count TextUnitSpans", err)
	}
	if storedSpans != int64(unitCount) {
		return corpus.Corpora{}, fmt.Errorf("%w: Corpus has orphan TextUnitSpans", corpus.ErrCorpusDataIntegrity)
	}
	return corpus.RestoreCorpora(corpus.Corpora{ID: id, Texts: chunkedTexts})
}

func corpusState(ctx context.Context, statements statements, zoneID string) (db.GetCorpusStateRow, error) {
	state, err := statements.GetCorpusState(ctx, zoneID)
	if errors.Is(err, sql.ErrNoRows) {
		return db.GetCorpusStateRow{}, nil
	}
	return state, err
}

func corpusReadable(state db.GetCorpusStateRow, id corpus.CorporaID) bool {
	if corpus.ValidateCorporaID(id) != nil {
		return false
	}
	sequence := corporaIDSequence(id)
	if sequence <= state.CurrentCorporaID {
		return true
	}
	return state.BuildingCorporaID.Valid && state.BuildingCorporaID.Int64 == sequence &&
		state.BuildingState.String == "prepared"
}

func corporaIDSequence(id corpus.CorporaID) int64 {
	sequence, _ := corpus.CorporaIDSequence(id)
	return sequence
}

func normalizeTextUnitIDs(ids []textunits.TextUnitID) []textunits.TextUnitID {
	unique := make(map[textunits.TextUnitID]struct{}, len(ids))
	for _, id := range ids {
		unique[id] = struct{}{}
	}
	result := make([]textunits.TextUnitID, 0, len(unique))
	for id := range unique {
		result = append(result, id)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func textUnitIDStrings(ids []textunits.TextUnitID) []string {
	result := make([]string, len(ids))
	for index, id := range ids {
		result[index] = string(id)
	}
	return result
}
