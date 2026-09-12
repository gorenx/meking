package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/memoria-space/meking/community/adapter/sqlite/internal/db"

	communityreport "github.com/memoria-space/meking/community/report"
)

var _ communityreport.Store = (*Store)(nil)

// Insert writes the Archive-validated generation batch in one immediate
// transaction. An existing ReportID rejects and rolls back the entire batch.
// Domain validation belongs to report.Archive and is not repeated here.
func (s *Store) Insert(
	ctx context.Context,
	reports []communityreport.Report,
) error {
	return s.write(ctx, func(statements statements) error {
		for _, report := range reports {
			if err := writeReport(ctx, statements, report); err != nil {
				return err
			}
		}
		return nil
	})
}

func writeReport(
	ctx context.Context,
	statements statements,
	report communityreport.Report,
) error {
	if _, err := statements.GetReport(ctx, string(report.ID)); err == nil {
		return fmt.Errorf(
			"%w: Report ID %q already exists",
			communityreport.ErrReportConflict,
			report.ID,
		)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return classifySQLite("read existing Report", err)
	}
	promptHash := make([]byte, len(report.Settings.PromptHash))
	copy(promptHash, report.Settings.PromptHash[:])
	if err := statements.CreateReport(ctx, db.CreateReportParams{
		ID:                string(report.ID),
		CommunityID:       string(report.CommunityID),
		Period:            report.Period,
		Title:             report.Title,
		Summary:           report.Summary,
		Rank:              report.Rank,
		RatingExplanation: report.RatingExplanation,
		FullContent:       report.FullContent,
		FullContentJson:   report.FullContentJSON,
		Model:             report.Settings.Model,
		PromptHash:        promptHash,
		Tokenizer:         report.Settings.Tokenizer,
		MaxInputTokens:    int64(report.Settings.MaxInputTokens),
		MaxReportLength:   int64(report.Settings.MaxReportLength),
	}); err != nil {
		return classifySQLite("create Report", err)
	}
	for ordinal, source := range report.EntitySources {
		if err := statements.AddReportEntity(ctx, db.AddReportEntityParams{
			ReportID: string(report.ID),
			Ordinal:  int64(ordinal),
			EntityID: source.ID,
			Version:  int64(source.Version),
		}); err != nil {
			return classifySQLite("add Report Entity source", err)
		}
	}
	for ordinal, source := range report.RelationSources {
		if err := statements.AddReportRelation(ctx, db.AddReportRelationParams{
			ReportID:   string(report.ID),
			Ordinal:    int64(ordinal),
			RelationID: source.ID,
			Version:    int64(source.Version),
		}); err != nil {
			return classifySQLite("add Report Relation source", err)
		}
	}
	for ordinal, source := range report.ClaimSources {
		if err := statements.AddReportClaimEvidence(
			ctx,
			db.AddReportClaimEvidenceParams{
				ReportID:      string(report.ID),
				Ordinal:       int64(ordinal),
				ClaimID:       source.ID,
				Version:       int64(source.Version),
				EvidenceIndex: int64(source.EvidenceIndex),
			},
		); err != nil {
			return classifySQLite("add Report Claim source", err)
		}
	}
	for ordinal, textUnitID := range report.TextUnitIDs {
		if err := statements.AddReportTextUnit(ctx, db.AddReportTextUnitParams{
			ReportID:   string(report.ID),
			Ordinal:    int64(ordinal),
			TextUnitID: textUnitID,
		}); err != nil {
			return classifySQLite("add Report TextUnit source", err)
		}
	}
	return nil
}
