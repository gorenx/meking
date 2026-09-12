package sqlite

import (
	"context"

	"github.com/memoria-space/meking/community/adapter/sqlite/internal/db"
)

// statements binds generated Community persistence to exactly one Zone.
type statements struct {
	queries *db.Queries
	zoneID  string
}

func newStatements(executor db.DBTX, zoneID string) statements {
	return statements{queries: db.New(executor), zoneID: zoneID}
}

func newGlobalStatements(executor db.DBTX) *db.Queries {
	return db.New(executor)
}

func (s statements) AddCommunity(ctx context.Context, arg db.AddCommunityParams) error {
	arg.ZoneID = s.zoneID
	return s.queries.AddCommunity(ctx, arg)
}

func (s statements) AddCommunityEntity(ctx context.Context, arg db.AddCommunityEntityParams) error {
	arg.ZoneID = s.zoneID
	return s.queries.AddCommunityEntity(ctx, arg)
}

func (s statements) AddCommunityMember(ctx context.Context, arg db.AddCommunityMemberParams) error {
	arg.ZoneID = s.zoneID
	return s.queries.AddCommunityMember(ctx, arg)
}

func (s statements) AddCommunityRelation(ctx context.Context, arg db.AddCommunityRelationParams) error {
	arg.ZoneID = s.zoneID
	return s.queries.AddCommunityRelation(ctx, arg)
}

func (s statements) AddReportClaimEvidence(ctx context.Context, arg db.AddReportClaimEvidenceParams) error {
	arg.ZoneID = s.zoneID
	return s.queries.AddReportClaimEvidence(ctx, arg)
}

func (s statements) AddReportEntity(ctx context.Context, arg db.AddReportEntityParams) error {
	arg.ZoneID = s.zoneID
	return s.queries.AddReportEntity(ctx, arg)
}

func (s statements) AddReportRelation(ctx context.Context, arg db.AddReportRelationParams) error {
	arg.ZoneID = s.zoneID
	return s.queries.AddReportRelation(ctx, arg)
}

func (s statements) AddReportSetReport(ctx context.Context, arg db.AddReportSetReportParams) error {
	arg.ZoneID = s.zoneID
	return s.queries.AddReportSetReport(ctx, arg)
}

func (s statements) AddReportTextUnit(ctx context.Context, arg db.AddReportTextUnitParams) error {
	arg.ZoneID = s.zoneID
	return s.queries.AddReportTextUnit(ctx, arg)
}

func (s statements) CreateCommunitySet(ctx context.Context, arg db.CreateCommunitySetParams) error {
	arg.ZoneID = s.zoneID
	return s.queries.CreateCommunitySet(ctx, arg)
}

func (s statements) CreateReport(ctx context.Context, arg db.CreateReportParams) error {
	arg.ZoneID = s.zoneID
	return s.queries.CreateReport(ctx, arg)
}

func (s statements) CreateReportSet(ctx context.Context, arg db.CreateReportSetParams) error {
	arg.ZoneID = s.zoneID
	return s.queries.CreateReportSet(ctx, arg)
}

func (s statements) FindCommunityWithoutParent(ctx context.Context, communitySetID string) (string, error) {
	return s.queries.FindCommunityWithoutParent(ctx, db.FindCommunityWithoutParentParams{
		ZoneID: s.zoneID, CommunitySetID: communitySetID,
	})
}

func (s statements) FindMemberWithoutCommunity(ctx context.Context, communitySetID string) (string, error) {
	return s.queries.FindMemberWithoutCommunity(ctx, db.FindMemberWithoutCommunityParams{
		ZoneID: s.zoneID, CommunitySetID: communitySetID,
	})
}

func (s statements) FindMemberWithoutEntity(ctx context.Context, communitySetID string) (string, error) {
	return s.queries.FindMemberWithoutEntity(ctx, db.FindMemberWithoutEntityParams{
		ZoneID: s.zoneID, CommunitySetID: communitySetID,
	})
}

func (s statements) GetCommunitySet(ctx context.Context, id string) (db.CommunitySet, error) {
	return s.queries.GetCommunitySet(ctx, db.GetCommunitySetParams{ZoneID: s.zoneID, ID: id})
}

func (s statements) GetReport(ctx context.Context, id string) (db.Report, error) {
	return s.queries.GetReport(ctx, db.GetReportParams{ZoneID: s.zoneID, ID: id})
}

func (s statements) GetReportSet(ctx context.Context, id string) (db.ReportSet, error) {
	return s.queries.GetReportSet(ctx, db.GetReportSetParams{ZoneID: s.zoneID, ID: id})
}

func (s statements) ListCommunities(ctx context.Context, communitySetID string) ([]db.ListCommunitiesRow, error) {
	return s.queries.ListCommunities(ctx, db.ListCommunitiesParams{
		ZoneID: s.zoneID, CommunitySetID: communitySetID,
	})
}

func (s statements) ListCommunityEntities(ctx context.Context, communitySetID string) ([]db.ListCommunityEntitiesRow, error) {
	return s.queries.ListCommunityEntities(ctx, db.ListCommunityEntitiesParams{
		ZoneID: s.zoneID, CommunitySetID: communitySetID,
	})
}

func (s statements) ListCommunityMembers(ctx context.Context, arg db.ListCommunityMembersParams) ([]db.ListCommunityMembersRow, error) {
	arg.ZoneID = s.zoneID
	return s.queries.ListCommunityMembers(ctx, arg)
}

func (s statements) ListCommunityRelations(ctx context.Context, communitySetID string) ([]db.ListCommunityRelationsRow, error) {
	return s.queries.ListCommunityRelations(ctx, db.ListCommunityRelationsParams{
		ZoneID: s.zoneID, CommunitySetID: communitySetID,
	})
}

func (s statements) ListReportClaimEvidence(ctx context.Context, reportID string) ([]db.ListReportClaimEvidenceRow, error) {
	return s.queries.ListReportClaimEvidence(ctx, db.ListReportClaimEvidenceParams{
		ZoneID: s.zoneID, ReportID: reportID,
	})
}

func (s statements) ListReportEntities(ctx context.Context, reportID string) ([]db.ListReportEntitiesRow, error) {
	return s.queries.ListReportEntities(ctx, db.ListReportEntitiesParams{ZoneID: s.zoneID, ReportID: reportID})
}

func (s statements) ListReportRelations(ctx context.Context, reportID string) ([]db.ListReportRelationsRow, error) {
	return s.queries.ListReportRelations(ctx, db.ListReportRelationsParams{ZoneID: s.zoneID, ReportID: reportID})
}

func (s statements) ListReportSetReports(ctx context.Context, reportSetID string) ([]db.ListReportSetReportsRow, error) {
	return s.queries.ListReportSetReports(ctx, db.ListReportSetReportsParams{
		ZoneID: s.zoneID, ReportSetID: reportSetID,
	})
}

func (s statements) ListReportTextUnits(ctx context.Context, reportID string) ([]db.ListReportTextUnitsRow, error) {
	return s.queries.ListReportTextUnits(ctx, db.ListReportTextUnitsParams{ZoneID: s.zoneID, ReportID: reportID})
}
