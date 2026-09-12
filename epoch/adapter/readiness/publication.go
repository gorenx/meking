// Package readiness validates the cross-context facts required to publish an Epoch.
package readiness

import (
	"context"
	"errors"
	"fmt"

	"github.com/memoria-space/meking/community"
	"github.com/memoria-space/meking/corpus"
	"github.com/memoria-space/meking/corpus/textunits"
	"github.com/memoria-space/meking/epoch"
	"github.com/memoria-space/meking/knowledge"
)

type Corpora interface {
	Corpora(context.Context, corpus.CorporaID) (corpus.Corpora, error)
}

type Structures interface {
	LoadStructure(context.Context, community.StructureID) (community.Structure, error)
}

type CommunitySets interface {
	Load(context.Context, community.CommunitySetID) (community.CommunitySet, error)
}

type EntityVectors interface {
	Validate(context.Context, []knowledge.Reference[knowledge.EntityID]) error
}

type TextUnitVectors interface {
	Validate(context.Context, string, []textunits.TextUnitBody) error
}

type PublicationReadiness struct {
	corpora         Corpora
	structures      Structures
	communitySets   CommunitySets
	entityVectors   EntityVectors
	textUnitVectors TextUnitVectors
}

func NewPublicationReadiness(
	corpora Corpora,
	structures Structures,
	communitySets CommunitySets,
	entityVectors EntityVectors,
	textUnitVectors TextUnitVectors,
) (*PublicationReadiness, error) {
	switch {
	case corpora == nil:
		return nil, errors.New("create Epoch publication readiness: Corpora are required")
	case structures == nil:
		return nil, errors.New("create Epoch publication readiness: Community Structures are required")
	case communitySets == nil:
		return nil, errors.New("create Epoch publication readiness: CommunitySets are required")
	case entityVectors == nil:
		return nil, errors.New("create Epoch publication readiness: Entity vectors are required")
	case textUnitVectors == nil:
		return nil, errors.New("create Epoch publication readiness: TextUnit vectors are required")
	}
	return &PublicationReadiness{
		corpora:         corpora,
		structures:      structures,
		communitySets:   communitySets,
		entityVectors:   entityVectors,
		textUnitVectors: textUnitVectors,
	}, nil
}

func (readiness *PublicationReadiness) Check(
	ctx context.Context,
	structureID epoch.StructureID,
	corporaID epoch.CorporaID,
	versions knowledge.Manifest,
) error {
	if readiness == nil {
		return errors.New("check Epoch publication readiness: adapter is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	structure, err := readiness.structures.LoadStructure(
		ctx,
		community.StructureID(structureID),
	)
	if err != nil {
		return fmt.Errorf("read Community Structure for Epoch: %w", err)
	}
	if structure.ID != community.StructureID(structureID) ||
		structure.CorporaID != string(corporaID) ||
		!structure.Knowledge.Equal(versions) {
		return errors.New("Community Structure does not match the Epoch boundary")
	}
	set, err := readiness.communitySets.Load(ctx, structure.CommunitySetID)
	if err != nil {
		return fmt.Errorf("read CommunitySet for Epoch: %w", err)
	}
	if set.ID != structure.CommunitySetID {
		return errors.New("CommunitySet does not match the Community Structure")
	}
	if err := readiness.entityVectors.Validate(
		ctx,
		versions.Entities,
	); err != nil {
		return fmt.Errorf("validate Epoch Entity vectors: %w", err)
	}
	setCorpora, err := readiness.corpora.Corpora(ctx, corpus.CorporaID(corporaID))
	if err != nil {
		return fmt.Errorf("read Epoch Corpora %q: %w", corporaID, err)
	}
	if setCorpora.ID != corpus.CorporaID(corporaID) {
		return fmt.Errorf(
			"Corpus returned Corpora %q; Epoch requires %q",
			setCorpora.ID,
			corporaID,
		)
	}
	if err := readiness.textUnitVectors.Validate(
		ctx,
		string(corporaID),
		corpusTextUnits(setCorpora),
	); err != nil {
		return fmt.Errorf("validate Epoch Corpora %q TextUnit vectors: %w", corporaID, err)
	}
	return nil
}

func corpusTextUnits(set corpus.Corpora) []textunits.TextUnitBody {
	spans := set.TextUnits()
	result := make([]textunits.TextUnitBody, 0, len(spans))
	for _, span := range spans {
		result = append(result, span.TextUnit)
	}
	return result
}
