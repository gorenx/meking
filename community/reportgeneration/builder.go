package reportgeneration

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/memoria-space/meking/community"
	communityreport "github.com/memoria-space/meking/community/report"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/transaction"
)

type Input struct {
	EpochID          int64
	StructureID      community.StructureID
	CorporaID        string
	Knowledge        knowledge.Manifest
	EpochPublishedAt time.Time
}

type Dependencies struct {
	Transactions  transaction.Tx
	Structures    Structures
	CommunitySets CommunitySets
	Knowledge     KnowledgeReader
	Corpus        CorpusReader
	Generator     Generator
	Reports       Reports
	ReportSets    ReportSets
	Publications  communityreport.PublicationStore
	Vectors       Vectors
}

type Builder struct {
	transactions  transaction.Tx
	structures    Structures
	communitySets CommunitySets
	knowledge     KnowledgeReader
	corpus        CorpusReader
	generator     Generator
	reports       Reports
	reportSets    ReportSets
	publications  communityreport.PublicationStore
	vectors       Vectors
}

func NewBuilder(dependencies Dependencies) (*Builder, error) {
	switch {
	case dependencies.Transactions == nil:
		return nil, errors.New("create Community Report Builder: Transactions are required")
	case dependencies.Structures == nil:
		return nil, errors.New("create Community Report Builder: Structures are required")
	case dependencies.CommunitySets == nil:
		return nil, errors.New("create Community Report Builder: CommunitySets are required")
	case dependencies.Knowledge == nil:
		return nil, errors.New("create Community Report Builder: Knowledge is required")
	case dependencies.Corpus == nil:
		return nil, errors.New("create Community Report Builder: Corpus is required")
	case dependencies.Generator == nil:
		return nil, errors.New("create Community Report Builder: Generator is required")
	case dependencies.Reports == nil:
		return nil, errors.New("create Community Report Builder: Reports are required")
	case dependencies.ReportSets == nil:
		return nil, errors.New("create Community Report Builder: ReportSets are required")
	case dependencies.Publications == nil:
		return nil, errors.New("create Community Report Builder: Publications are required")
	}
	return &Builder{
		transactions:  dependencies.Transactions,
		structures:    dependencies.Structures,
		communitySets: dependencies.CommunitySets,
		knowledge:     dependencies.Knowledge,
		corpus:        dependencies.Corpus,
		generator:     dependencies.Generator,
		reports:       dependencies.Reports,
		reportSets:    dependencies.ReportSets,
		publications:  dependencies.Publications,
		vectors:       dependencies.Vectors,
	}, nil
}

func (builder *Builder) Build(
	ctx context.Context,
	input Input,
) (communityreport.Publication, error) {
	if builder == nil {
		return communityreport.Publication{}, errors.New("build Community Reports: Builder is required")
	}
	if err := validateInput(input); err != nil {
		return communityreport.Publication{}, err
	}
	publication, err := builder.publications.LoadReportPublication(ctx, input.StructureID)
	switch {
	case err == nil:
		if publication.EpochID != input.EpochID {
			return communityreport.Publication{}, communityreport.ErrPublicationConflict
		}
	case errors.Is(err, communityreport.ErrPublicationNotFound):
		publication, err = builder.prepare(ctx, input)
		if err != nil {
			return communityreport.Publication{}, err
		}
	default:
		return communityreport.Publication{}, err
	}
	if publication.VectorsReady {
		return publication, nil
	}
	reportSet, err := builder.reportSets.LoadReportSet(ctx, publication.ReportSetID)
	if err != nil {
		return communityreport.Publication{}, err
	}
	reports, err := builder.reports.Reports(ctx, orderedReportIDs(reportSet))
	if err != nil {
		return communityreport.Publication{}, err
	}
	if builder.vectors != nil {
		if _, err := builder.vectors.Build(ctx, reportSet, reports); err != nil {
			return communityreport.Publication{}, fmt.Errorf("build Community Report vectors: %w", err)
		}
	}
	err = builder.transactions.WithTx(ctx, func(ctx context.Context) error {
		return builder.publications.CompleteReportPublication(
			ctx,
			publication.StructureID,
			publication.ReportSetID,
		)
	})
	if err != nil {
		return communityreport.Publication{}, fmt.Errorf("complete Community Report publication: %w", err)
	}
	publication.VectorsReady = true
	return publication, nil
}

func (builder *Builder) prepare(
	ctx context.Context,
	input Input,
) (communityreport.Publication, error) {
	structure, err := builder.structures.LoadStructure(ctx, input.StructureID)
	if err != nil {
		return communityreport.Publication{}, err
	}
	if structure.CorporaID != input.CorporaID ||
		!structure.Knowledge.Equal(input.Knowledge) {
		return communityreport.Publication{}, communityreport.ErrPublicationConflict
	}
	set, err := builder.communitySets.Load(ctx, structure.CommunitySetID)
	if err != nil {
		return communityreport.Publication{}, err
	}
	knowledgeSnapshot, err := builder.knowledge.ReportSnapshot(ctx, input.Knowledge)
	if err != nil {
		return communityreport.Publication{}, err
	}
	corpus, err := builder.corpus.Corpora(ctx, input.CorporaID)
	if err != nil {
		return communityreport.Publication{}, err
	}
	if err := validateCorpus(corpus, input.CorporaID); err != nil {
		return communityreport.Publication{}, err
	}
	reports, err := builder.generator.Generate(ctx, reportInput(
		set.Communities,
		knowledgeSnapshot,
		corpus.TextUnitIDs,
		input.EpochPublishedAt.Format(time.DateOnly),
	))
	if err != nil {
		return communityreport.Publication{}, err
	}
	reportSet, err := communityreport.NewReportSet(
		set.ID,
		corpus.ID,
		reports,
		textUnitSet(corpus.TextUnitIDs),
	)
	if err != nil {
		return communityreport.Publication{}, err
	}
	publication := communityreport.Publication{
		EpochID:      input.EpochID,
		StructureID:  input.StructureID,
		ReportSetID:  reportSet.ID,
		VectorsReady: false,
		CreatedAt:    reportSet.CreatedAt,
	}
	err = builder.transactions.WithTx(ctx, func(ctx context.Context) error {
		if err := builder.reports.Save(ctx, reports); err != nil {
			return err
		}
		if err := builder.reportSets.SaveReportSet(ctx, reportSet); err != nil {
			return err
		}
		return builder.publications.PrepareReportPublication(ctx, publication)
	})
	if err != nil {
		return communityreport.Publication{}, fmt.Errorf("prepare Community Report publication: %w", err)
	}
	return publication, nil
}

func validateInput(input Input) error {
	if input.EpochID <= 0 ||
		strings.TrimSpace(input.CorporaID) == "" ||
		input.EpochPublishedAt.IsZero() ||
		input.EpochPublishedAt.Location() != time.UTC {
		return communityreport.ErrPublicationConflict
	}
	if err := input.Knowledge.Validate(); err != nil {
		return communityreport.ErrPublicationConflict
	}
	_, err := community.RestoreStructureID(input.StructureID)
	return err
}

func validateCorpus(snapshot CorpusSnapshot, expectedID string) error {
	if strings.TrimSpace(snapshot.ID) == "" ||
		snapshot.ID != expectedID ||
		len(snapshot.TextUnitIDs) == 0 {
		return communityreport.ErrPublicationConflict
	}
	previous := ""
	for _, id := range snapshot.TextUnitIDs {
		if strings.TrimSpace(id) == "" || previous != "" && id <= previous {
			return communityreport.ErrPublicationConflict
		}
		previous = id
	}
	return nil
}

func orderedReportIDs(set communityreport.ReportSet) []communityreport.ID {
	communityIDs := make([]community.CommunityID, 0, len(set.Reports))
	for communityID := range set.Reports {
		communityIDs = append(communityIDs, communityID)
	}
	sort.Slice(communityIDs, func(left int, right int) bool {
		return communityIDs[left] < communityIDs[right]
	})
	result := make([]communityreport.ID, len(communityIDs))
	for index, communityID := range communityIDs {
		result[index] = set.Reports[communityID]
	}
	return result
}

func textUnitSet(ids []string) map[string]struct{} {
	result := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		result[id] = struct{}{}
	}
	return result
}
