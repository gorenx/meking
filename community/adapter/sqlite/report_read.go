package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/community"
	communityreport "github.com/memoria-space/meking/community/report"
	"github.com/memoria-space/meking/zone"
)

// LoadReports reconstructs an ordered immutable Report batch in one deferred
// read transaction. The caller-supplied IDs are already validated and unique at
// the Archive boundary; the adapter repeats identity checks because Store is a
// public persistence contract and must reject invalid direct calls.
func (s *Store) LoadReports(
	ctx context.Context,
	ids []communityreport.ID,
) ([]communityreport.Report, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf(
			"%w: Report read batch must not be empty",
			communityreport.ErrInvalidReport,
		)
	}
	seen := make(map[communityreport.ID]struct{}, len(ids))
	for _, id := range ids {
		if err := communityreport.ValidateID(id); err != nil {
			return nil, err
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf(
				"%w: duplicate Report ID %q in read batch",
				communityreport.ErrInvalidReport,
				id,
			)
		}
		seen[id] = struct{}{}
	}
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return nil, fmt.Errorf("read Community Reports: %w", err)
	}
	transaction, err := s.database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, classifySQLite("begin Report read", err)
	}
	defer transaction.Rollback()
	statements := newStatements(transaction, string(zoneID))
	reports := make([]communityreport.Report, len(ids))
	for index, id := range ids {
		report, err := loadReport(ctx, statements, id)
		if err != nil {
			return nil, err
		}
		reports[index] = report
	}
	return reports, nil
}

func loadReport(
	ctx context.Context,
	statements statements,
	id communityreport.ID,
) (communityreport.Report, error) {
	row, err := statements.GetReport(ctx, string(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return communityreport.Report{}, communityreport.ErrReportNotFound
		}
		return communityreport.Report{}, classifySQLite("read Report", err)
	}
	if len(row.PromptHash) != 32 {
		return communityreport.Report{}, fmt.Errorf(
			"%w: Report %q PromptHash has %d bytes",
			communityreport.ErrReportDataIntegrity,
			id,
			len(row.PromptHash),
		)
	}
	var promptHash [32]byte
	copy(promptHash[:], row.PromptHash)

	var draft community.ReportDraft
	if err := json.Unmarshal([]byte(row.FullContentJson), &draft); err != nil {
		return communityreport.Report{}, fmt.Errorf(
			"%w: Report %q FullContentJSON is invalid: %v",
			communityreport.ErrReportDataIntegrity,
			id,
			err,
		)
	}
	entities, err := loadReportEntities(ctx, statements, id)
	if err != nil {
		return communityreport.Report{}, err
	}
	relations, err := loadReportRelations(ctx, statements, id)
	if err != nil {
		return communityreport.Report{}, err
	}
	claims, err := loadReportClaims(ctx, statements, id)
	if err != nil {
		return communityreport.Report{}, err
	}
	textUnitIDs, err := loadReportTextUnits(ctx, statements, id)
	if err != nil {
		return communityreport.Report{}, err
	}
	report := communityreport.Report{
		ID:                communityreport.ID(row.ID),
		CommunityID:       community.CommunityID(row.CommunityID),
		Period:            row.Period,
		Title:             row.Title,
		Summary:           row.Summary,
		Findings:          append([]community.ReportFinding(nil), draft.Findings...),
		Rank:              row.Rank,
		RatingExplanation: row.RatingExplanation,
		FullContent:       row.FullContent,
		FullContentJSON:   row.FullContentJson,
		EntitySources:     entities,
		RelationSources:   relations,
		ClaimSources:      claims,
		TextUnitIDs:       textUnitIDs,
		Settings: communityreport.Settings{
			Model:           row.Model,
			PromptHash:      promptHash,
			Tokenizer:       row.Tokenizer,
			MaxInputTokens:  int(row.MaxInputTokens),
			MaxReportLength: int(row.MaxReportLength),
		},
	}
	if err := communityreport.ValidateReport(report); err != nil {
		return communityreport.Report{}, fmt.Errorf(
			"%w: Report %q is invalid: %v",
			communityreport.ErrReportDataIntegrity,
			id,
			err,
		)
	}
	return report, nil
}

func loadReportEntities(
	ctx context.Context,
	statements statements,
	id communityreport.ID,
) ([]community.EntityReference, error) {
	rows, err := statements.ListReportEntities(ctx, string(id))
	if err != nil {
		return nil, classifySQLite("list Report Entity sources", err)
	}
	result := make([]community.EntityReference, len(rows))
	for index, row := range rows {
		if row.Ordinal != int64(index) {
			return nil, reportOrdinalError(id, "Entity", index, row.Ordinal)
		}
		result[index] = community.EntityReference{
			ID:      row.EntityID,
			Version: uint64(row.Version),
		}
	}
	return result, nil
}

func loadReportRelations(
	ctx context.Context,
	statements statements,
	id communityreport.ID,
) ([]community.RelationReference, error) {
	rows, err := statements.ListReportRelations(ctx, string(id))
	if err != nil {
		return nil, classifySQLite("list Report Relation sources", err)
	}
	result := make([]community.RelationReference, len(rows))
	for index, row := range rows {
		if row.Ordinal != int64(index) {
			return nil, reportOrdinalError(id, "Relation", index, row.Ordinal)
		}
		result[index] = community.RelationReference{
			ID:      row.RelationID,
			Version: uint64(row.Version),
		}
	}
	return result, nil
}

func loadReportClaims(
	ctx context.Context,
	statements statements,
	id communityreport.ID,
) ([]communityreport.ClaimSource, error) {
	rows, err := statements.ListReportClaimEvidence(ctx, string(id))
	if err != nil {
		return nil, classifySQLite("list Report Claim sources", err)
	}
	result := make([]communityreport.ClaimSource, len(rows))
	for index, row := range rows {
		if row.Ordinal != int64(index) {
			return nil, reportOrdinalError(id, "Claim", index, row.Ordinal)
		}
		result[index] = communityreport.ClaimSource{
			ID:            row.ClaimID,
			Version:       uint64(row.Version),
			EvidenceIndex: int(row.EvidenceIndex),
		}
	}
	return result, nil
}

func loadReportTextUnits(
	ctx context.Context,
	statements statements,
	id communityreport.ID,
) ([]string, error) {
	rows, err := statements.ListReportTextUnits(ctx, string(id))
	if err != nil {
		return nil, classifySQLite("list Report TextUnit sources", err)
	}
	result := make([]string, len(rows))
	for index, row := range rows {
		if row.Ordinal != int64(index) {
			return nil, reportOrdinalError(id, "TextUnit", index, row.Ordinal)
		}
		result[index] = row.TextUnitID
	}
	return result, nil
}

func reportOrdinalError(
	id communityreport.ID,
	source string,
	expected int,
	actual int64,
) error {
	return fmt.Errorf(
		"%w: Report %q %s source ordinal is %d; expected %d",
		communityreport.ErrReportDataIntegrity,
		id,
		source,
		actual,
		expected,
	)
}
