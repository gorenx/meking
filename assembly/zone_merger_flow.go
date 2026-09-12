package assembly

import (
	"context"
	"fmt"

	"github.com/memoria-space/meking/corpus"
	corpusmerger "github.com/memoria-space/meking/corpus/zonemerger"
	"github.com/memoria-space/meking/zone"
	zonemerger "github.com/memoria-space/meking/zone/merger"
)

type corpusMergeAdapter struct {
	application *corpusmerger.BoundaryApplication
	corpora     *corpus.Service
}

func (adapter corpusMergeAdapter) ValidateChildCorpora(
	ctx context.Context,
	sourceCorporaID string,
) error {
	childZoneID, err := zone.RequireChildID(ctx)
	if err != nil {
		return err
	}
	sourceContext, err := zone.RouteContext(ctx, childZoneID)
	if err != nil {
		return err
	}
	id, err := corpus.ParseCorporaID(sourceCorporaID)
	if err != nil {
		return err
	}
	if _, err := adapter.corpora.Corpora(sourceContext, id); err != nil {
		return fmt.Errorf("read stable Child Corpora %q: %w", sourceCorporaID, err)
	}
	return nil
}

func (adapter corpusMergeAdapter) MergeChildCorpora(
	ctx context.Context,
	sourceCorporaID string,
) error {
	return adapter.application.MergeChildCorpora(ctx, sourceCorporaID)
}

func openZoneMergerFlow(
	zones zoneApplications,
	corpora corpusApplications,
	knowledgeApplications knowledgeApplications,
) (*zonemerger.Application, error) {
	return zonemerger.New(zonemerger.Dependencies{
		Zones: zones.catalog,
		Corpus: corpusMergeAdapter{
			application: corpora.boundaries, corpora: corpora.service,
		},
		Knowledge: knowledgeApplications.merger,
	})
}
