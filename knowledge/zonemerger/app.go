package zonemerger

import (
	"errors"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/resolution"
	"github.com/memoria-space/meking/knowledge/submission"
	"github.com/memoria-space/meking/transaction"
)

type Dependencies struct {
	Tx               transaction.Tx
	Conflicts        Conflicts
	Provenance       Provenance
	Identities       Identities
	CurrentVersions  knowledge.CurrentVersions
	CurrentKnowledge *knowledge.Reader
	Submissions      *submission.Application
	Resolutions      *resolution.Application
}

type Application struct {
	Dependencies
}

func New(dependencies Dependencies) (*Application, error) {
	switch {
	case dependencies.Tx == nil:
		return nil, errors.New("create Knowledge Zone Merger: transaction is required")
	case dependencies.Conflicts == nil:
		return nil, errors.New("create Knowledge Zone Merger: Conflicts are required")
	case dependencies.Provenance == nil:
		return nil, errors.New("create Knowledge Zone Merger: Provenance is required")
	case dependencies.Identities == nil:
		return nil, errors.New("create Knowledge Zone Merger: Knowledge identities are required")
	case dependencies.CurrentVersions == nil:
		return nil, errors.New("create Knowledge Zone Merger: Current Knowledge versions are required")
	case dependencies.CurrentKnowledge == nil:
		return nil, errors.New("create Knowledge Zone Merger: Current Knowledge is required")
	case dependencies.Submissions == nil:
		return nil, errors.New("create Knowledge Zone Merger: Submissions are required")
	case dependencies.Resolutions == nil:
		return nil, errors.New("create Knowledge Zone Merger: Resolutions are required")
	default:
		return &Application{
			Dependencies: dependencies,
		}, nil
	}
}
