package report

import (
	"fmt"
	"math"
	"strings"

	"github.com/memoria-space/meking/community"
	"github.com/memoria-space/meking/internal/uuid"
)

// ValidateReport verifies immutable identity, complete structured projections,
// source identities and order, TextUnit closure, and generation settings.
func ValidateReport(report Report) error {
	if err := ValidateID(report.ID); err != nil {
		return err
	}
	return validateReportContent(report)
}

// validateReportContent verifies every immutable field except ID so Generator
// can reject invalid model output and source closure before allocating identity.
func validateReportContent(report Report) error {
	if err := community.ValidateCommunityID(report.CommunityID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidReport, err)
	}
	if strings.TrimSpace(report.Period) == "" {
		return fmt.Errorf("%w: Report Period is required", ErrInvalidReport)
	}
	draft := community.ReportDraft{
		Title:             report.Title,
		Summary:           report.Summary,
		Findings:          report.Findings,
		Rating:            report.Rank,
		RatingExplanation: report.RatingExplanation,
	}
	if err := validateReportDraft(draft); err != nil {
		return err
	}
	if report.FullContent != community.RenderFullReport(draft) {
		return fmt.Errorf("%w: FullContent does not match structured Report fields", ErrInvalidReport)
	}
	fullJSON, err := community.MarshalReportJSON(draft)
	if err != nil {
		return fmt.Errorf("%w: render FullContentJSON: %v", ErrInvalidReport, err)
	}
	if report.FullContentJSON != fullJSON {
		return fmt.Errorf("%w: FullContentJSON does not match structured Report fields", ErrInvalidReport)
	}
	if err := validateReportSources(report); err != nil {
		return err
	}
	if err := validateReportSettings(report.Settings); err != nil {
		return err
	}
	return nil
}

func validateReportDraft(draft community.ReportDraft) error {
	if strings.TrimSpace(draft.Title) == "" ||
		strings.TrimSpace(draft.Summary) == "" ||
		strings.TrimSpace(draft.RatingExplanation) == "" {
		return fmt.Errorf(
			"%w: Title, Summary, and RatingExplanation are required",
			ErrInvalidReport,
		)
	}
	if math.IsNaN(draft.Rating) || math.IsInf(draft.Rating, 0) {
		return fmt.Errorf("%w: Rank must be finite", ErrInvalidReport)
	}
	for index, finding := range draft.Findings {
		if strings.TrimSpace(finding.Summary) == "" ||
			strings.TrimSpace(finding.Explanation) == "" {
			return fmt.Errorf(
				"%w: Finding %d requires Summary and Explanation",
				ErrInvalidReport,
				index,
			)
		}
	}
	return nil
}

func validateReportSources(report Report) error {
	if len(report.EntitySources) == 0 {
		return fmt.Errorf("%w: Report requires at least one Entity source", ErrInvalidReport)
	}
	seenEntities := make(map[string]struct{}, len(report.EntitySources))
	for _, source := range report.EntitySources {
		if !uuid.IsCanonicalV4(source.ID) {
			return fmt.Errorf(
				"%w: Entity source ID %q is not a canonical lowercase UUID v4",
				ErrInvalidReport,
				source.ID,
			)
		}
		if err := validateReportVersion(source.Version, "Entity", source.ID); err != nil {
			return err
		}
		if _, duplicate := seenEntities[source.ID]; duplicate {
			return fmt.Errorf("%w: duplicate Entity source %q", ErrInvalidReport, source.ID)
		}
		seenEntities[source.ID] = struct{}{}
	}
	seenRelations := make(map[string]struct{}, len(report.RelationSources))
	for _, source := range report.RelationSources {
		if !uuid.IsCanonicalV4(source.ID) {
			return fmt.Errorf(
				"%w: Relation source ID %q is not a canonical lowercase UUID v4",
				ErrInvalidReport,
				source.ID,
			)
		}
		if err := validateReportVersion(source.Version, "Relation", source.ID); err != nil {
			return err
		}
		if _, duplicate := seenRelations[source.ID]; duplicate {
			return fmt.Errorf("%w: duplicate Relation source %q", ErrInvalidReport, source.ID)
		}
		seenRelations[source.ID] = struct{}{}
	}
	seenClaims := make(map[ClaimSource]struct{}, len(report.ClaimSources))
	claimVersions := make(map[string]uint64)
	for index, source := range report.ClaimSources {
		if !uuid.IsCanonicalV4(source.ID) {
			return fmt.Errorf(
				"%w: Claim source ID %q is not a canonical lowercase UUID v4",
				ErrInvalidReport,
				source.ID,
			)
		}
		if err := validateReportVersion(source.Version, "Claim", source.ID); err != nil {
			return err
		}
		if source.EvidenceIndex < 0 {
			return fmt.Errorf(
				"%w: Claim source %q EvidenceIndex must be non-negative",
				ErrInvalidReport,
				source.ID,
			)
		}
		if version, found := claimVersions[source.ID]; found && version != source.Version {
			return fmt.Errorf(
				"%w: Claim source %q uses more than one Version",
				ErrInvalidReport,
				source.ID,
			)
		}
		claimVersions[source.ID] = source.Version
		if _, duplicate := seenClaims[source]; duplicate {
			return fmt.Errorf(
				"%w: duplicate Claim source %q Statement %d",
				ErrInvalidReport,
				source.ID,
				source.EvidenceIndex,
			)
		}
		if index > 0 {
			previous := report.ClaimSources[index-1]
			if previous.ID > source.ID ||
				previous.ID == source.ID &&
					previous.EvidenceIndex >= source.EvidenceIndex {
				return fmt.Errorf(
					"%w: Claim sources must be ordered by ID and EvidenceIndex",
					ErrInvalidReport,
				)
			}
		}
		seenClaims[source] = struct{}{}
	}
	if _, err := validateAvailableTextUnits(report.TextUnitIDs); err != nil {
		return err
	}
	return nil
}

func validateReportVersion(version uint64, object string, id string) error {
	if version == 0 || version > math.MaxInt64 {
		return fmt.Errorf(
			"%w: %s source %q Version must fit a positive SQLite INTEGER",
			ErrInvalidReport,
			object,
			id,
		)
	}
	return nil
}

func validateAvailableTextUnits(ids []string) (map[string]struct{}, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf(
			"%w: Report requires at least one TextUnit",
			ErrInvalidReport,
		)
	}
	result := make(map[string]struct{}, len(ids))
	for index, id := range ids {
		if len(id) != 128 {
			return nil, fmt.Errorf(
				"%w: TextUnit ID %q is not canonical lowercase SHA-512",
				ErrInvalidReport,
				id,
			)
		}
		for _, character := range []byte(id) {
			if !isLowerHex(character) {
				return nil, fmt.Errorf(
					"%w: TextUnit ID %q is not canonical lowercase SHA-512",
					ErrInvalidReport,
					id,
				)
			}
		}
		if index > 0 && ids[index-1] >= id {
			return nil, fmt.Errorf(
				"%w: TextUnit IDs must be strictly ordered",
				ErrInvalidReport,
			)
		}
		result[id] = struct{}{}
	}
	return result, nil
}

func isLowerHex(character byte) bool {
	return character >= '0' && character <= '9' ||
		character >= 'a' && character <= 'f'
}

func validateReportSettings(settings Settings) error {
	if strings.TrimSpace(settings.Model) == "" ||
		strings.TrimSpace(settings.Tokenizer) == "" {
		return fmt.Errorf("%w: Report Model and Tokenizer are required", ErrInvalidReport)
	}
	if settings.PromptHash == ([32]byte{}) {
		return fmt.Errorf("%w: Report PromptHash is required", ErrInvalidReport)
	}
	if settings.MaxInputTokens <= 0 || settings.MaxReportLength <= 0 {
		return fmt.Errorf(
			"%w: Report token and length limits must be positive",
			ErrInvalidReport,
		)
	}
	return nil
}

func newReport(
	communityID community.CommunityID,
	period string,
	draft community.ReportDraft,
	entities []community.EntityReference,
	relations []community.RelationReference,
	claims []ClaimSource,
	textUnitIDs []string,
	settings Settings,
) (Report, error) {
	fullJSON, err := community.MarshalReportJSON(draft)
	if err != nil {
		return Report{}, fmt.Errorf("marshal structured Report: %w", err)
	}
	report := Report{
		CommunityID:       communityID,
		Period:            period,
		Title:             draft.Title,
		Summary:           draft.Summary,
		Findings:          append([]community.ReportFinding(nil), draft.Findings...),
		Rank:              draft.Rating,
		RatingExplanation: draft.RatingExplanation,
		FullContent:       community.RenderFullReport(draft),
		FullContentJSON:   fullJSON,
		EntitySources:     append([]community.EntityReference(nil), entities...),
		RelationSources:   append([]community.RelationReference(nil), relations...),
		ClaimSources:      append([]ClaimSource(nil), claims...),
		TextUnitIDs:       append([]string(nil), textUnitIDs...),
		Settings:          settings,
	}
	if err := validateReportContent(report); err != nil {
		return Report{}, err
	}
	report.ID, err = newReportID()
	if err != nil {
		return Report{}, fmt.Errorf("create Report ID: %w", err)
	}
	if err := ValidateID(report.ID); err != nil {
		return Report{}, err
	}
	return report, nil
}
