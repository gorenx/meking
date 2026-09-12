package sqlite

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/adapter/sqlite/internal/db"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/zone"
)

func (s *Store) BeginCorporaBuild(ctx context.Context) (corpus.CorporaID, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return "", err
	}
	zoneIDValue := string(zoneID)
	var corporaID corpus.CorporaID
	err = s.write(ctx, zoneIDValue, func(statements statements) error {
		if err := statements.EnsureCorpusState(ctx, zoneIDValue); err != nil {
			return classifySQLite("initialize Corpus state", err)
		}
		state, err := statements.GetCorpusState(ctx, zoneIDValue)
		if err != nil {
			return classifySQLite("read Corpus state", err)
		}
		if !state.BuildingCorporaID.Valid {
			updated, err := statements.StartCorpusBuild(ctx, zoneIDValue)
			if err != nil {
				return classifySQLite("start Corpus build", err)
			}
			if updated != 1 {
				return corpus.ErrCorporaPreparationConflict
			}
			state, err = statements.GetCorpusState(ctx, zoneIDValue)
			if err != nil {
				return classifySQLite("read started Corpus build", err)
			}
			if state.CurrentCorporaID > 0 {
				if err := statements.CopyTextUnitSpans(ctx, db.CopyTextUnitSpansParams{
					ZoneID:          zoneIDValue,
					TargetCorporaID: state.BuildingCorporaID.Int64,
					SourceCorporaID: state.CurrentCorporaID,
				}); err != nil {
					return classifySQLite("copy prior Corpus TextUnitSpans", err)
				}
			}
		}
		if !state.BuildingCorporaID.Valid || state.BuildingState.String != "building" {
			if !state.BuildingCorporaID.Valid || state.BuildingState.String != "prepared" {
				return corpus.ErrCorpusDataIntegrity
			}
		}
		corporaID, err = corpus.NewCorporaID(state.BuildingCorporaID.Int64)
		if err != nil {
			return corpus.ErrCorpusDataIntegrity
		}
		return nil
	})
	return corporaID, err
}

func (s *Store) OpenTextChunkingProgress(
	ctx context.Context,
	corporaID corpus.CorporaID,
	sourceEventID corpus.EventID,
	textID text.ID,
) (corpus.TextChunkingProgress, error) {
	if corpus.ValidateCorporaID(corporaID) != nil || sourceEventID == "" || text.ValidateID(textID) != nil {
		return corpus.TextChunkingProgress{}, corpus.ErrInvalidCorpus
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return corpus.TextChunkingProgress{}, err
	}
	zoneIDValue := string(zoneID)
	var result corpus.TextChunkingProgress
	err = s.write(ctx, zoneIDValue, func(statements statements) error {
		row, err := statements.GetTextChunkingProgressBySource(ctx, db.GetTextChunkingProgressBySourceParams{
			ZoneID: zoneIDValue, SourceEventID: string(sourceEventID),
		})
		if err == nil {
			if row.TextID != string(textID) {
				return corpus.ErrCorporaPreparationConflict
			}
			result, err = restoreTextChunkingProgress(row.TextID, row.NextChunkIndex, row.Completed)
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return classifySQLite("read Text chunking progress", err)
		}
		state, err := statements.GetCorpusState(ctx, zoneIDValue)
		if err != nil {
			return classifySQLite("read Corpus state", err)
		}
		if !state.BuildingCorporaID.Valid || state.BuildingCorporaID.Int64 != corporaIDSequence(corporaID) {
			return corpus.ErrCorporaPreparationConflict
		}
		if state.BuildingState.String == "prepared" {
			return corpus.ErrCorporaAwaitingActivation
		}
		if state.BuildingState.String != "building" {
			return corpus.ErrCorpusDataIntegrity
		}
		if err := statements.DeleteCorporaTextUnitSpans(ctx, db.DeleteCorporaTextUnitSpansParams{
			ZoneID: zoneIDValue, CorporaID: corporaIDSequence(corporaID), TextID: string(textID),
		}); err != nil {
			return classifySQLite("clear Building Corpus Text spans", err)
		}
		if err := statements.CreateTextChunkingProgress(ctx, db.CreateTextChunkingProgressParams{
			ZoneID: zoneIDValue, TextID: string(textID), SourceEventID: string(sourceEventID),
		}); err != nil {
			return classifySQLite("create Text chunking progress", err)
		}
		result = corpus.TextChunkingProgress{TextID: textID}
		return nil
	})
	return result, err
}

func (s *Store) SaveTextUnitBatch(
	ctx context.Context,
	corporaID corpus.CorporaID,
	textID text.ID,
	spans []textunits.TextUnit,
) error {
	if corpus.ValidateCorporaID(corporaID) != nil || text.ValidateID(textID) != nil || len(spans) == 0 {
		return corpus.ErrInvalidCorpus
	}
	prepared := append([]textunits.TextUnit(nil), spans...)
	slices.SortFunc(prepared, func(left, right textunits.TextUnit) int {
		if order := cmp.Compare(left.StartIndex, right.StartIndex); order != 0 {
			return order
		}
		return cmp.Compare(left.EndIndex, right.EndIndex)
	})
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return err
	}
	zoneIDValue := string(zoneID)
	return s.write(ctx, zoneIDValue, func(statements statements) error {
		state, err := statements.GetCorpusState(ctx, zoneIDValue)
		if err != nil {
			return classifySQLite("read Corpus state", err)
		}
		if !state.BuildingCorporaID.Valid || state.BuildingCorporaID.Int64 != corporaIDSequence(corporaID) ||
			state.BuildingState.String != "building" {
			return corpus.ErrCorporaPreparationConflict
		}
		value, err := loadText(ctx, statements, zoneIDValue, textID)
		if err != nil {
			return err
		}
		for index, span := range prepared {
			if index > 0 && span.StartIndex == prepared[index-1].StartIndex &&
				span.EndIndex == prepared[index-1].EndIndex {
				return corpus.ErrInvalidCorpus
			}
			if err := corpus.ValidateTextUnitContent(value, span); err != nil {
				return err
			}
			if err := saveTextUnit(ctx, statements, zoneIDValue, span.TextUnit); err != nil {
				return err
			}
			if err := statements.AddTextUnitSpan(ctx, db.AddTextUnitSpanParams{
				ZoneID: zoneIDValue, CorporaID: corporaIDSequence(corporaID), TextID: string(textID),
				TextUnitID: string(span.TextUnit.ID), StartChar: int64(span.StartIndex),
				EndChar: int64(span.EndIndex), TokenCount: int64(span.TokenCount),
			}); err != nil {
				return classifySQLite("add Building Corpus TextUnitSpan", err)
			}
		}
		return nil
	})
}

func (s *Store) AdvanceTextChunkingProgress(
	ctx context.Context,
	corporaID corpus.CorporaID,
	progress corpus.TextChunkingProgress,
	nextChunkIndex int,
	complete bool,
) error {
	if corpus.ValidateCorporaID(corporaID) != nil || text.ValidateID(progress.TextID) != nil ||
		progress.NextChunkIndex < 0 || progress.Completed || nextChunkIndex <= progress.NextChunkIndex {
		return corpus.ErrInvalidCorpus
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return err
	}
	zoneIDValue := string(zoneID)
	return s.write(ctx, zoneIDValue, func(statements statements) error {
		state, err := statements.GetCorpusState(ctx, zoneIDValue)
		if err != nil {
			return classifySQLite("read Corpus state", err)
		}
		if !state.BuildingCorporaID.Valid ||
			state.BuildingCorporaID.Int64 != corporaIDSequence(corporaID) ||
			state.BuildingState.String != "building" {
			return corpus.ErrCorporaPreparationConflict
		}
		completed := int64(0)
		if complete {
			completed = 1
		}
		updated, err := statements.AdvanceTextChunkingProgress(ctx, db.AdvanceTextChunkingProgressParams{
			NextChunkIndex: int64(nextChunkIndex), Completed: completed,
			ZoneID: zoneIDValue, TextID: string(progress.TextID),
			ExpectedNextChunkIndex: int64(progress.NextChunkIndex),
		})
		if err != nil {
			return classifySQLite("advance Text chunking progress", err)
		}
		if updated != 1 {
			return corpus.ErrCorporaPreparationConflict
		}
		return nil
	})
}

func (s *Store) FinalizeCorpora(
	ctx context.Context,
	id corpus.CorporaID,
) (corpus.Corpora, bool, error) {
	if err := corpus.ValidateCorporaID(id); err != nil {
		return corpus.Corpora{}, false, err
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return corpus.Corpora{}, false, err
	}
	zoneIDValue := string(zoneID)
	var result corpus.Corpora
	ready := false
	err = s.write(ctx, zoneIDValue, func(statements statements) error {
		state, err := statements.GetCorpusState(ctx, zoneIDValue)
		if err != nil {
			return classifySQLite("read Corpus state", err)
		}
		if state.CurrentCorporaID == corporaIDSequence(id) {
			result, err = loadCorporaContent(ctx, statements, zoneIDValue, id)
			return err
		}
		if !state.BuildingCorporaID.Valid || state.BuildingCorporaID.Int64 != corporaIDSequence(id) {
			return corpus.ErrCorporaNotFound
		}
		if state.BuildingState.String == "prepared" {
			result, err = loadCorporaContent(ctx, statements, zoneIDValue, id)
			return err
		}
		if state.BuildingState.String != "building" {
			return corpus.ErrCorpusDataIntegrity
		}
		incomplete, err := statements.CountIncompleteTextChunking(ctx, zoneIDValue)
		if err != nil {
			return classifySQLite("count incomplete Text chunking", err)
		}
		if incomplete != 0 {
			return nil
		}
		coveredTextCount, err := statements.CountCorporaTexts(ctx, db.CountCorporaTextsParams{
			ZoneID: zoneIDValue, CorporaID: corporaIDSequence(id),
		})
		if err != nil {
			return classifySQLite("count Building Corpus Texts", err)
		}
		if coveredTextCount == 0 {
			return nil
		}
		result, err = loadCorporaContent(ctx, statements, zoneIDValue, id)
		if err != nil {
			return err
		}
		updated, err := statements.MarkCorpusPrepared(ctx, db.MarkCorpusPreparedParams{
			ZoneID:            zoneIDValue,
			BuildingCorporaID: sql.NullInt64{Int64: corporaIDSequence(id), Valid: true},
		})
		if err != nil {
			return classifySQLite("mark Corpus prepared", err)
		}
		if updated != 1 {
			return corpus.ErrCorporaPreparationConflict
		}
		ready = true
		return nil
	})
	return result, ready, err
}

func restoreTextChunkingProgress(
	textIDValue string,
	nextChunkIndex int64,
	completed int64,
) (corpus.TextChunkingProgress, error) {
	result := corpus.TextChunkingProgress{
		TextID: text.ID(textIDValue), NextChunkIndex: int(nextChunkIndex), Completed: completed == 1,
	}
	if text.ValidateID(result.TextID) != nil || nextChunkIndex < 0 ||
		nextChunkIndex > int64(^uint(0)>>1) || completed < 0 || completed > 1 {
		return corpus.TextChunkingProgress{}, fmt.Errorf("%w: invalid Text chunking progress", corpus.ErrCorpusDataIntegrity)
	}
	return result, nil
}
