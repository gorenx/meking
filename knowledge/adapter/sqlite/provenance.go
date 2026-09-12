package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/adapter/sqlite/internal/db"
	"github.com/memoria-space/meking/knowledge/deletion"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/knowledge/resolution"
	"github.com/memoria-space/meking/knowledge/submission"
)

type entityMetadata struct {
	Frequency int `json:"frequency"`
}

type relationMetadata struct {
	Weight float64 `json:"weight"`
}

type claimMetadata struct {
	Records []claimRecord `json:"records"`
}

type claimRecord struct {
	SubjectText string `json:"subject_text"`
	ObjectText  string `json:"object_text"`
	Status      string `json:"status"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	SourceText  string `json:"source_text"`
}
type sourceEvidence struct {
	ZoneID     string         `json:"zone_id"`
	TextUnitID string         `json:"text_unit_id"`
	Source     evidenceSource `json:"source"`
}

type evidenceSource struct {
	Kind      string `json:"kind"`
	CorporaID string `json:"corpora_id,omitempty"`
	MessageID string `json:"message_id,omitempty"`
}

type sourceSupport struct {
	SourceID        string
	KnowledgeKind   string
	KnowledgeID     string
	Version         knowledge.Version
	CandidateNumber uint64
	MetadataJSON    string
	Evidence        []provenance.Evidence
}

func (database *Database) Source(ctx context.Context, sourceID string) (provenance.Source, bool, error) {
	queries, err := database.reader(ctx)
	if err != nil {
		return provenance.Source{}, false, err
	}
	row, err := queries.GetKnowledgeSource(ctx, db.GetKnowledgeSourceParams{ZoneID: queries.zoneID, SourceID: sourceID})
	if errors.Is(err, sql.ErrNoRows) {
		return provenance.Source{}, false, nil
	}
	if err != nil {
		return provenance.Source{}, false, classifySQLite("read Knowledge Source", err)
	}
	result := provenance.Source{ID: row.SourceID, Kind: row.SourceKind, ProducerID: row.ProducerID}
	if err := provenance.Validate(result); err != nil {
		return provenance.Source{}, false, fmt.Errorf("%w: stored Knowledge Source %q: %v", knowledge.ErrDataIntegrity, sourceID, err)
	}
	return result, true, nil
}

func (database *Database) RecordSource(ctx context.Context, source provenance.Source, evidence []provenance.Evidence) error {
	if err := provenance.Validate(source); err != nil {
		return err
	}
	evidence = provenance.CanonicalEvidence(evidence)
	if err := provenance.ValidateEvidence(evidence); err != nil {
		return err
	}
	encoded, err := encodeEvidence(evidence)
	if err != nil {
		return err
	}
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	if err := queries.CreateKnowledgeSource(ctx, db.CreateKnowledgeSourceParams{
		ZoneID: queries.zoneID, SourceID: source.ID, SourceKind: source.Kind, ProducerID: source.ProducerID,
	}); err != nil {
		if constraint(err) {
			return fmt.Errorf("%w: Source ID %q is already in use", provenance.ErrInvalidSource, source.ID)
		}
		return classifySQLite("record Knowledge Source", err)
	}
	if err := queries.SaveKnowledgeEvidence(ctx, db.SaveKnowledgeEvidenceParams{
		ZoneID: queries.zoneID, SourceID: source.ID, EvidenceJson: encoded,
	}); err != nil {
		return classifySQLite("save Knowledge Evidence", err)
	}
	return nil
}

func (database *Database) ConfirmEntity(ctx context.Context, value provenance.EntityConfirmation) error {
	if err := validateEntityConfirmation(value); err != nil {
		return err
	}
	metadata, err := encodeEntityMetadata(value.Metadata.Frequency)
	if err != nil {
		return err
	}
	return database.saveSourceSupport(ctx, sourceSupport{SourceID: value.SourceID, KnowledgeKind: "entity",
		KnowledgeID: string(value.EntityID), Version: value.Version, MetadataJSON: metadata, Evidence: value.Metadata.Evidence})
}
func (database *Database) ConfirmRelation(ctx context.Context, value provenance.RelationConfirmation) error {
	if err := validateRelationConfirmation(value); err != nil {
		return err
	}
	metadata, err := encodeRelationMetadata(value.Metadata.Weight)
	if err != nil {
		return err
	}
	return database.saveSourceSupport(ctx, sourceSupport{SourceID: value.SourceID, KnowledgeKind: "relation",
		KnowledgeID: string(value.RelationID), Version: value.Version, MetadataJSON: metadata, Evidence: value.Metadata.Evidence})
}
func (database *Database) ConfirmClaim(ctx context.Context, value provenance.ClaimConfirmation) error {
	if err := validateClaimConfirmation(value); err != nil {
		return err
	}
	metadata, err := encodeClaimMetadata(value.Metadata)
	if err != nil {
		return err
	}
	return database.saveSourceSupport(ctx, sourceSupport{SourceID: value.SourceID, KnowledgeKind: "claim",
		KnowledgeID: string(value.ClaimID), Version: value.Version, MetadataJSON: metadata, Evidence: claimEvidence(value.Metadata)})
}

func (database *Database) ReadEntityEvidence(
	ctx context.Context,
	reference knowledge.Reference[knowledge.EntityID],
) ([]provenance.EntityConfirmation, error) {
	queries, err := database.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListEntityVersionSources(ctx, db.ListEntityVersionSourcesParams{
		ZoneID:   queries.zoneID,
		EntityID: string(reference.ID),
		Version:  int64(reference.Version),
	})
	if err != nil {
		return nil, classifySQLite("list Entity Version Sources", err)
	}
	result := make([]provenance.EntityConfirmation, 0, len(rows))
	for _, row := range rows {
		metadata, err := decodeEntityMetadata(row.MetadataJson)
		if err != nil {
			return nil, err
		}
		evidence, err := database.sourceEvidence(ctx, queries, row.SourceID)
		if err != nil {
			return nil, err
		}
		result = append(result, provenance.EntityConfirmation{
			EntityID: reference.ID,
			Version:  reference.Version,
			SourceID: row.SourceID,
			Metadata: provenance.EntityMetadata{
				Frequency: metadata.Frequency,
				Evidence:  evidence,
			},
		})
	}
	return result, nil
}

func (database *Database) ReadRelationEvidence(
	ctx context.Context,
	reference knowledge.Reference[knowledge.RelationID],
) ([]provenance.RelationConfirmation, error) {
	queries, err := database.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListRelationVersionSources(ctx, db.ListRelationVersionSourcesParams{
		ZoneID:     queries.zoneID,
		RelationID: string(reference.ID),
		Version:    int64(reference.Version),
	})
	if err != nil {
		return nil, classifySQLite("list Relation Version Sources", err)
	}
	result := make([]provenance.RelationConfirmation, 0, len(rows))
	for _, row := range rows {
		metadata, err := decodeRelationMetadata(row.MetadataJson)
		if err != nil {
			return nil, err
		}
		evidence, err := database.sourceEvidence(ctx, queries, row.SourceID)
		if err != nil {
			return nil, err
		}
		result = append(result, provenance.RelationConfirmation{
			RelationID: reference.ID,
			Version:    reference.Version,
			SourceID:   row.SourceID,
			Metadata: provenance.RelationMetadata{
				Weight:   metadata.Weight,
				Evidence: evidence,
			},
		})
	}
	return result, nil
}

func (database *Database) ReadClaimEvidence(
	ctx context.Context,
	reference knowledge.Reference[knowledge.ClaimID],
) ([]provenance.ClaimConfirmation, error) {
	queries, err := database.reader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListClaimVersionSources(ctx, db.ListClaimVersionSourcesParams{
		ZoneID:  queries.zoneID,
		ClaimID: string(reference.ID),
		Version: int64(reference.Version),
	})
	if err != nil {
		return nil, classifySQLite("list Claim Version Sources", err)
	}
	result := make([]provenance.ClaimConfirmation, 0, len(rows))
	for _, row := range rows {
		metadata, err := decodeClaimMetadata(row.MetadataJson)
		if err != nil {
			return nil, err
		}
		evidence, err := database.sourceEvidence(ctx, queries, row.SourceID)
		if err != nil {
			return nil, err
		}
		result = append(result, provenance.ClaimConfirmation{
			ClaimID:  reference.ID,
			Version:  reference.Version,
			SourceID: row.SourceID,
			Metadata: claimMetadataValues(metadata.Records, evidence),
		})
	}
	return result, nil
}

func (database *Database) SubmissionResult(ctx context.Context, sourceID string) (submission.Result, error) {
	queries, err := database.reader(ctx)
	if err != nil {
		return submission.Result{}, err
	}
	result := submission.Result{SourceID: sourceID}
	entities, err := queries.ListSourceEntityVersions(ctx, db.ListSourceEntityVersionsParams{ZoneID: queries.zoneID, SourceID: sourceID})
	if err != nil {
		return result, classifySQLite("read Entity Submission result", err)
	}
	for _, row := range entities {
		ref := knowledge.Reference[knowledge.EntityID]{ID: knowledge.EntityID(row.EntityID), Version: knowledge.Version(row.Version)}
		if row.CreatedVersion {
			result.CreatedVersions.Entities = append(result.CreatedVersions.Entities, ref)
		} else {
			result.ConfirmedVersions.Entities = append(result.ConfirmedVersions.Entities, ref)
		}
	}
	relations, err := queries.ListSourceRelationVersions(ctx, db.ListSourceRelationVersionsParams{ZoneID: queries.zoneID, SourceID: sourceID})
	if err != nil {
		return result, classifySQLite("read Relation Submission result", err)
	}
	for _, row := range relations {
		ref := knowledge.Reference[knowledge.RelationID]{ID: knowledge.RelationID(row.RelationID), Version: knowledge.Version(row.Version)}
		if row.CreatedVersion {
			result.CreatedVersions.Relations = append(result.CreatedVersions.Relations, ref)
		} else {
			result.ConfirmedVersions.Relations = append(result.ConfirmedVersions.Relations, ref)
		}
	}
	claims, err := queries.ListSourceClaimVersions(ctx, db.ListSourceClaimVersionsParams{ZoneID: queries.zoneID, SourceID: sourceID})
	if err != nil {
		return result, classifySQLite("read Claim Submission result", err)
	}
	for _, row := range claims {
		ref := knowledge.Reference[knowledge.ClaimID]{ID: knowledge.ClaimID(row.ClaimID), Version: knowledge.Version(row.Version)}
		if row.CreatedVersion {
			result.CreatedVersions.Claims = append(result.CreatedVersions.Claims, ref)
		} else {
			result.ConfirmedVersions.Claims = append(result.ConfirmedVersions.Claims, ref)
		}
	}
	if err := database.addCandidateResults(ctx, queries, sourceID, &result); err != nil {
		return submission.Result{}, err
	}
	return result, nil
}

func (database *Database) ResolutionResult(ctx context.Context, sourceID string) (resolution.Result, error) {
	version, created, found, err := database.directResult(ctx, sourceID)
	if err != nil {
		return resolution.Result{}, err
	}
	if !found {
		return resolution.Result{}, fmt.Errorf("%w: Resolution Source %q has no result", knowledge.ErrDataIntegrity, sourceID)
	}
	return resolution.Result{Version: version, CreatedVersion: created}, nil
}
func (database *Database) DeletionResult(ctx context.Context, sourceID string) (deletion.Result, error) {
	version, created, found, err := database.directResult(ctx, sourceID)
	if err != nil {
		return deletion.Result{}, err
	}
	if !found {
		return deletion.Result{}, fmt.Errorf("%w: Deletion Source %q has no result", knowledge.ErrDataIntegrity, sourceID)
	}
	return deletion.Result{Version: version, CreatedVersion: created}, nil
}

func (database *Database) directResult(ctx context.Context, sourceID string) (knowledge.Version, bool, bool, error) {
	queries, err := database.reader(ctx)
	if err != nil {
		return 0, false, false, err
	}
	type directVersion struct {
		version int64
		created bool
	}
	var values []directVersion
	entities, err := queries.ListSourceEntityVersions(ctx, db.ListSourceEntityVersionsParams{ZoneID: queries.zoneID, SourceID: sourceID})
	if err != nil {
		return 0, false, false, err
	}
	for _, row := range entities {
		values = append(values, directVersion{row.Version, row.CreatedVersion})
	}
	relations, err := queries.ListSourceRelationVersions(ctx, db.ListSourceRelationVersionsParams{ZoneID: queries.zoneID, SourceID: sourceID})
	if err != nil {
		return 0, false, false, err
	}
	for _, row := range relations {
		values = append(values, directVersion{row.Version, row.CreatedVersion})
	}
	claims, err := queries.ListSourceClaimVersions(ctx, db.ListSourceClaimVersionsParams{ZoneID: queries.zoneID, SourceID: sourceID})
	if err != nil {
		return 0, false, false, err
	}
	for _, row := range claims {
		values = append(values, directVersion{row.Version, row.CreatedVersion})
	}
	if len(values) == 0 {
		return 0, false, false, nil
	}
	if len(values) != 1 {
		return 0, false, false, fmt.Errorf("%w: Source %q has %d formal results", knowledge.ErrDataIntegrity, sourceID, len(values))
	}
	return knowledge.Version(values[0].version), values[0].created, true, nil
}

func (database *Database) saveSourceSupport(ctx context.Context, support sourceSupport) error {
	queries, err := database.writer(ctx)
	if err != nil {
		return err
	}
	storedEvidence, err := database.sourceEvidence(ctx, queries, support.SourceID)
	if err != nil {
		return err
	}
	if !slices.Equal(storedEvidence, provenance.CanonicalEvidence(support.Evidence)) {
		return fmt.Errorf("%w: Knowledge metadata Evidence differs from Source %q", knowledge.ErrDataIntegrity, support.SourceID)
	}
	if err := queries.SaveKnowledgeSourceMetadata(ctx, db.SaveKnowledgeSourceMetadataParams{ZoneID: queries.zoneID, SourceID: support.SourceID,
		KnowledgeKind: support.KnowledgeKind, KnowledgeID: support.KnowledgeID, Version: int64(support.Version),
		CandidateNumber: int64(support.CandidateNumber), MetadataJson: support.MetadataJSON}); err != nil {
		return classifySQLite("save Knowledge Source metadata", err)
	}
	return nil
}

func (database *Database) sourceEvidence(ctx context.Context, queries statements, sourceID string) ([]provenance.Evidence, error) {
	encoded, err := queries.GetKnowledgeEvidence(ctx, db.GetKnowledgeEvidenceParams{ZoneID: queries.zoneID, SourceID: sourceID})
	if err != nil {
		return nil, classifySQLite("read Knowledge Evidence", err)
	}
	var stored []sourceEvidence
	if err := json.Unmarshal([]byte(encoded), &stored); err != nil {
		return nil, fmt.Errorf("%w: decode Knowledge Evidence: %v", knowledge.ErrDataIntegrity, err)
	}
	result := make([]provenance.Evidence, len(stored))
	for i, value := range stored {
		var source provenance.EvidenceSource
		switch value.Source.Kind {
		case "corpora":
			source = provenance.CorporaSource{CorporaID: value.Source.CorporaID}
		case "message":
			source = provenance.MessageSource{MessageID: value.Source.MessageID}
		default:
			return nil, fmt.Errorf("%w: unsupported Knowledge Evidence source %q", knowledge.ErrDataIntegrity, value.Source.Kind)
		}
		result[i] = provenance.Evidence{ZoneID: value.ZoneID, TextUnitID: value.TextUnitID, Source: source}
	}
	if err := provenance.ValidateEvidence(result); err != nil {
		return nil, fmt.Errorf("%w: stored Knowledge Evidence: %v", knowledge.ErrDataIntegrity, err)
	}
	return result, nil
}

func encodeEvidence(values []provenance.Evidence) (string, error) {
	stored := make([]sourceEvidence, len(values))
	for i, value := range values {
		var source evidenceSource
		switch evidence := value.Source.(type) {
		case provenance.CorporaSource:
			source = evidenceSource{Kind: "corpora", CorporaID: evidence.CorporaID}
		case provenance.MessageSource:
			source = evidenceSource{Kind: "message", MessageID: evidence.MessageID}
		default:
			return "", fmt.Errorf("encode Knowledge Evidence: unsupported source %T", value.Source)
		}
		stored[i] = sourceEvidence{ZoneID: value.ZoneID, TextUnitID: value.TextUnitID, Source: source}
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return "", fmt.Errorf("encode Knowledge Evidence: %w", err)
	}
	return string(encoded), nil
}
func encodeEntityMetadata(frequency int) (string, error) {
	encoded, err := json.Marshal(entityMetadata{Frequency: frequency})
	return encodedMetadata(encoded, err)
}
func encodeRelationMetadata(weight float64) (string, error) {
	encoded, err := json.Marshal(relationMetadata{Weight: weight})
	return encodedMetadata(encoded, err)
}
func encodeClaimMetadata(values []provenance.ClaimMetadata) (string, error) {
	encoded, err := json.Marshal(claimMetadata{Records: claimRecords(values)})
	return encodedMetadata(encoded, err)
}
func encodedMetadata(encoded []byte, err error) (string, error) {
	if err != nil {
		return "", fmt.Errorf("encode Knowledge Source metadata: %w", err)
	}
	return string(encoded), nil
}

func decodeEntityMetadata(encoded string) (entityMetadata, error) {
	var value entityMetadata
	if err := json.Unmarshal([]byte(encoded), &value); err != nil {
		return entityMetadata{}, fmt.Errorf("%w: decode Entity Source metadata: %v", knowledge.ErrDataIntegrity, err)
	}
	return value, nil
}
func decodeRelationMetadata(encoded string) (relationMetadata, error) {
	var value relationMetadata
	if err := json.Unmarshal([]byte(encoded), &value); err != nil {
		return relationMetadata{}, fmt.Errorf("%w: decode Relation Source metadata: %v", knowledge.ErrDataIntegrity, err)
	}
	return value, nil
}
func decodeClaimMetadata(encoded string) (claimMetadata, error) {
	var value claimMetadata
	if err := json.Unmarshal([]byte(encoded), &value); err != nil {
		return claimMetadata{}, fmt.Errorf("%w: decode Claim Source metadata: %v", knowledge.ErrDataIntegrity, err)
	}
	return value, nil
}
func claimRecords(values []provenance.ClaimMetadata) []claimRecord {
	result := make([]claimRecord, len(values))
	for i, value := range values {
		result[i] = claimRecord{SubjectText: value.SubjectText, ObjectText: value.ObjectText, Status: value.Status, StartDate: value.StartDate, EndDate: value.EndDate, SourceText: value.SourceText}
	}
	return result
}
func claimMetadataValues(values []claimRecord, evidence []provenance.Evidence) []provenance.ClaimMetadata {
	result := make([]provenance.ClaimMetadata, len(values))
	for i, value := range values {
		result[i] = provenance.ClaimMetadata{SubjectText: value.SubjectText, ObjectText: value.ObjectText, Status: value.Status, StartDate: value.StartDate, EndDate: value.EndDate, SourceText: value.SourceText, Evidence: append([]provenance.Evidence(nil), evidence...)}
	}
	return result
}
func claimEvidence(values []provenance.ClaimMetadata) []provenance.Evidence {
	if len(values) == 0 {
		return nil
	}
	return values[0].Evidence
}

func validateEntityConfirmation(value provenance.EntityConfirmation) error {
	if err := knowledge.ValidateEntityID(value.EntityID); err != nil {
		return err
	}
	if err := validateConfirmation(value.Version, value.SourceID); err != nil {
		return err
	}
	return provenance.ValidateEntityMetadata(value.Metadata)
}
func validateRelationConfirmation(value provenance.RelationConfirmation) error {
	if err := knowledge.ValidateRelationID(value.RelationID); err != nil {
		return err
	}
	if err := validateConfirmation(value.Version, value.SourceID); err != nil {
		return err
	}
	return provenance.ValidateRelationMetadata(value.Metadata)
}
func validateClaimConfirmation(value provenance.ClaimConfirmation) error {
	if err := knowledge.ValidateClaimID(value.ClaimID); err != nil {
		return err
	}
	if err := validateConfirmation(value.Version, value.SourceID); err != nil {
		return err
	}
	var evidence []provenance.Evidence
	for i, metadata := range value.Metadata {
		if err := provenance.ValidateClaimMetadata(metadata); err != nil {
			return err
		}
		if i == 0 {
			evidence = provenance.CanonicalEvidence(metadata.Evidence)
		} else if !slices.Equal(evidence, provenance.CanonicalEvidence(metadata.Evidence)) {
			return fmt.Errorf("%w: Claim metadata must share Source Evidence", knowledge.ErrInvalidChange)
		}
	}
	return nil
}
func validateConfirmation(version knowledge.Version, sourceID string) error {
	if sourceID == "" {
		return fmt.Errorf("%w: Knowledge Source ID is required", knowledge.ErrInvalidChange)
	}
	return knowledge.ValidateVersion(version)
}
