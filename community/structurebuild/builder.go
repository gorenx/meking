package structurebuild

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/memoria-space/meking/community"
	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/transaction"
)

type Input struct {
	SourceEventID string
	CorrelationID string
	CorporaID     string
	Knowledge     knowledge.Manifest
}

type Dependencies struct {
	Transactions            transaction.Tx
	Knowledge               KnowledgeReader
	CommunitySets           CommunitySets
	Structures              Structures
	Producer                Producer
	Detection               community.DetectConfig
	RelationChangeThreshold uint64
}

type Builder struct {
	transactions            transaction.Tx
	knowledge               KnowledgeReader
	communitySets           CommunitySets
	structures              Structures
	producer                Producer
	detection               community.DetectConfig
	relationChangeThreshold uint64
}

func NewBuilder(dependencies Dependencies) (*Builder, error) {
	switch {
	case dependencies.Transactions == nil:
		return nil, errors.New("create Community Structure Builder: Transactions are required")
	case dependencies.Knowledge == nil:
		return nil, errors.New("create Community Structure Builder: Knowledge is required")
	case dependencies.CommunitySets == nil:
		return nil, errors.New("create Community Structure Builder: CommunitySets are required")
	case dependencies.Structures == nil:
		return nil, errors.New("create Community Structure Builder: Structures are required")
	case dependencies.Producer == nil:
		return nil, errors.New("create Community Structure Builder: Producer is required")
	case dependencies.RelationChangeThreshold == 0 ||
		dependencies.RelationChangeThreshold > math.MaxInt64:
		return nil, errors.New("create Community Structure Builder: Relation change threshold must fit a positive SQLite INTEGER")
	}
	if err := dependencies.Detection.Validate(); err != nil {
		return nil, fmt.Errorf("create Community Structure Builder: invalid Detection configuration: %w", err)
	}
	return &Builder{
		transactions:            dependencies.Transactions,
		knowledge:               dependencies.Knowledge,
		communitySets:           dependencies.CommunitySets,
		structures:              dependencies.Structures,
		producer:                dependencies.Producer,
		detection:               dependencies.Detection,
		relationChangeThreshold: dependencies.RelationChangeThreshold,
	}, nil
}

func (builder *Builder) Build(
	ctx context.Context,
	input Input,
) (community.Structure, error) {
	if builder == nil {
		return community.Structure{}, errors.New("build Community Structure: Builder is required")
	}
	if err := validateInput(input); err != nil {
		return community.Structure{}, err
	}
	existing, err := builder.structures.LoadStructureAt(
		ctx,
		input.CorporaID,
		input.Knowledge,
	)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, community.ErrStructureNotFound) {
		return community.Structure{}, err
	}

	state, err := builder.structures.LoadStructureState(ctx)
	if err != nil {
		return community.Structure{}, err
	}
	previousStructure, err := builder.previousStructure(ctx, state)
	if err != nil {
		return community.Structure{}, err
	}
	var previousSet *community.CommunitySet
	previousKnowledge := knowledge.Manifest{}
	if previousStructure != nil {
		set, err := builder.communitySets.Load(ctx, previousStructure.CommunitySetID)
		if err != nil {
			return community.Structure{}, err
		}
		previousSet = &set
		previousKnowledge = previousStructure.Knowledge
	}
	snapshot, err := builder.knowledge.Snapshot(ctx, input.Knowledge)
	if err != nil {
		return community.Structure{}, err
	}
	relationChanges := changedRelations(previousKnowledge.Relations, input.Knowledge.Relations)
	accumulated, overflow := addCount(state.PendingRelationChanges, relationChanges)
	if overflow {
		return community.Structure{}, fmt.Errorf(
			"%w: Relation change count overflow",
			community.ErrInvalidStructure,
		)
	}
	set, rebuilt, err := builder.communitySet(ctx, previousSet, snapshot, accumulated)
	if err != nil {
		return community.Structure{}, err
	}
	structure, err := community.NewStructure(
		set.ID,
		input.CorporaID,
		input.Knowledge,
	)
	if err != nil {
		return community.Structure{}, err
	}
	pending := accumulated
	if rebuilt {
		pending = 0
	}
	nextState := community.StructureState{
		StructureID:            structure.ID,
		PendingRelationChanges: pending,
	}
	err = builder.transactions.WithTx(ctx, func(ctx context.Context) error {
		if rebuilt {
			if err := builder.communitySets.Save(ctx, set); err != nil {
				return err
			}
		}
		if err := builder.structures.SaveStructure(ctx, structure); err != nil {
			return err
		}
		if err := builder.structures.SaveStructureState(ctx, state, nextState); err != nil {
			return err
		}
		return builder.producer.PublishStructure(ctx, Prepared{
			SourceEventID: input.SourceEventID,
			CorrelationID: input.CorrelationID,
			Structure:     structure,
		})
	})
	if err != nil {
		return community.Structure{}, fmt.Errorf("commit Community Structure: %w", err)
	}
	return structure, nil
}

func (builder *Builder) previousStructure(
	ctx context.Context,
	state community.StructureState,
) (*community.Structure, error) {
	if state.StructureID == "" {
		return nil, nil
	}
	structure, err := builder.structures.LoadStructure(ctx, state.StructureID)
	if err != nil {
		return nil, err
	}
	return &structure, nil
}

func (builder *Builder) communitySet(
	ctx context.Context,
	previous *community.CommunitySet,
	snapshot KnowledgeSnapshot,
	accumulated uint64,
) (community.CommunitySet, bool, error) {
	rebuild := previous == nil ||
		previous.DetectorVersion != community.CurrentDetectorVersion ||
		previous.DetectionConfig != builder.detection ||
		accumulated >= builder.relationChangeThreshold
	if !rebuild {
		return *previous, false, nil
	}
	graph, err := projectGraph(snapshot.Entities, snapshot.Relations)
	if err != nil {
		return community.CommunitySet{}, false, err
	}
	set, err := community.Detect(ctx, graph, builder.detection)
	if err != nil {
		return community.CommunitySet{}, false, err
	}
	return set, true, nil
}

func validateInput(input Input) error {
	if strings.TrimSpace(string(input.SourceEventID)) == "" ||
		strings.TrimSpace(string(input.CorrelationID)) == "" ||
		strings.TrimSpace(input.CorporaID) == "" {
		return community.ErrInvalidStructure
	}
	if err := input.Knowledge.Validate(); err != nil {
		return community.ErrInvalidStructure
	}
	return nil
}

func changedRelations(
	previous []knowledge.Reference[knowledge.RelationID],
	current []knowledge.Reference[knowledge.RelationID],
) uint64 {
	var changed uint64
	left := 0
	right := 0
	for left < len(previous) && right < len(current) {
		switch {
		case previous[left].ID < current[right].ID:
			changed++
			left++
		case previous[left].ID > current[right].ID:
			changed++
			right++
		default:
			if previous[left].Version != current[right].Version {
				changed++
			}
			left++
			right++
		}
	}
	return changed + uint64(len(previous)-left) + uint64(len(current)-right)
}

func addCount(left uint64, right uint64) (uint64, bool) {
	if math.MaxUint64-left < right {
		return 0, true
	}
	return left + right, false
}
