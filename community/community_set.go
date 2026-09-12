package community

import (
	"fmt"
	"sort"
	"time"
)

// CommunitySetID identifies one immutable, complete Community hierarchy. It is
// a canonical lowercase UUID v4 and has no ordering or knowledge-version meaning.
type CommunitySetID string

// CommunityID identifies one exact Entity membership set. It is the lowercase
// SHA-256 of the canonical members and therefore remains stable across rebuilds
// only while the complete member set remains unchanged.
type CommunityID string

// EntityReference identifies one exact Knowledge Entity version consumed by a
// Community-owned result. ID is a canonical Knowledge EntityID string; Version
// is positive and only comparable within that Entity's own MVCC chain. The
// containing object defines ordering and whether the reference came from
// Community detection or Report generation.
type EntityReference struct {
	// ID is the canonical lowercase Knowledge Entity UUID selected by the
	// containing CommunitySet or Report.
	ID string
	// Version is positive, immutable, and only comparable within ID's Entity chain.
	Version uint64
}

// RelationReference identifies one exact Knowledge Relation version consumed
// by a Community-owned result. ID is a canonical Knowledge RelationID string;
// Version is positive and only comparable within that Relation's own MVCC chain.
// The containing object defines its ordering and use.
type RelationReference struct {
	// ID is the canonical lowercase Knowledge Relation UUID selected by the
	// containing CommunitySet or Report.
	ID string
	// Version is positive, immutable, and only comparable within ID's Relation chain.
	Version uint64
}

// Membership is one stable Entity set and its position in a complete
// CommunitySet hierarchy. It contains no Claim, TextUnit, report, or display data.
type Membership struct {
	// ID is derived from EntityIDs by the CommunityID length-prefixed SHA-256
	// algorithm and is stable only while the complete membership is unchanged.
	ID CommunityID
	// Number is the zero-based detected Community number within one CommunitySet.
	// It is contiguous but is not stable across rebuilds.
	Number int
	// Level is zero for roots and increases along every parent-child path.
	Level int
	// ParentID is nil for roots and otherwise identifies one CommunityMembership
	// in the same CommunitySet. Children are derived and never stored separately.
	ParentID *CommunityID
	// Final is true only when this membership has no finer child Community.
	Final bool
	// Unsplittable is true when another size-driven Leiden pass returned this
	// same membership instead of multiple strict child sets.
	Unsplittable bool
	// EntityIDs is the non-empty, strictly byte-sorted, duplicate-free set of
	// canonical Knowledge EntityID strings contained by this Community.
	EntityIDs []string
}

// CommunitySet is one immutable, fully validated Community hierarchy together
// with the exact Knowledge Entity and Relation versions read for its rebuild.
// Current query visibility belongs to ReportSet and is not stored here.
type CommunitySet struct {
	// ID is allocated before persistence and never reused for another set.
	ID CommunitySetID
	// DetectorVersion comes from CurrentDetectorVersion when the candidate is detected.
	// It lets a later derivation task distinguish a reusable clustering baseline
	// from one produced by an older normalization or Leiden contract.
	DetectorVersion uint32
	// DetectionConfig is the exact validated MaxClusterSize, LCC selection, and
	// Seed supplied to the detector that produced Communities. It is immutable
	// and compared with the next requested configuration before reuse.
	DetectionConfig DetectConfig
	// Communities is ordered by Level and then Number and contains the complete
	// parent-child hierarchy produced by one rebuild.
	Communities []Membership
	// Entities contains every active Entity row read for detection, ordered by ID
	// without duplicates. Isolated Entities may be absent from Communities because
	// the current detector derives nodes only from Relation endpoints.
	Entities []EntityReference
	// Relations contains every active Relation row projected into detection,
	// ordered by ID without duplicates.
	Relations []RelationReference
	// CreatedAt is the UTC completion time assigned after hierarchy validation.
	// It is diagnostic and does not participate in Community or knowledge identity.
	CreatedAt time.Time
}

// NewCommunitySet validates one detected hierarchy, canonicalizes copied
// Knowledge references and memberships, allocates a set ID, and records the
// current UTC completion time without performing persistence.
func NewCommunitySet(
	hierarchy Hierarchy,
	entities []EntityReference,
	relations []RelationReference,
	config DetectConfig,
) (CommunitySet, error) {
	id, err := newCommunitySetID()
	if err != nil {
		return CommunitySet{}, fmt.Errorf("create CommunitySet ID: %w", err)
	}
	return newCommunitySet(
		id,
		CurrentDetectorVersion,
		config,
		hierarchy,
		entities,
		relations,
		time.Now().UTC(),
	)
}

func newCommunitySet(
	id CommunitySetID,
	detectorVersion uint32,
	config DetectConfig,
	hierarchy Hierarchy,
	entities []EntityReference,
	relations []RelationReference,
	createdAt time.Time,
) (CommunitySet, error) {
	if err := hierarchy.Validate(); err != nil {
		return CommunitySet{}, fmt.Errorf("%w: %v", ErrInvalidCommunitySet, err)
	}
	result := CommunitySet{
		ID:              id,
		DetectorVersion: detectorVersion,
		DetectionConfig: config,
		Entities:        append([]EntityReference(nil), entities...),
		Relations:       append([]RelationReference(nil), relations...),
		CreatedAt:       createdAt.UTC(),
	}
	sort.Slice(result.Entities, func(left, right int) bool {
		return result.Entities[left].ID < result.Entities[right].ID
	})
	sort.Slice(result.Relations, func(left, right int) bool {
		return result.Relations[left].ID < result.Relations[right].ID
	})

	communityIDs := make(map[int]CommunityID, len(hierarchy.Communities))
	members := make(map[int][]string, len(hierarchy.Communities))
	for _, detected := range hierarchy.Communities {
		entityIDs := append([]string(nil), detected.Nodes...)
		sort.Strings(entityIDs)
		id := communityID(entityIDs)
		if _, duplicate := communityIDs[detected.ID]; duplicate {
			return CommunitySet{}, fmt.Errorf(
				"%w: duplicate detected Community number %d",
				ErrInvalidCommunitySet,
				detected.ID,
			)
		}
		communityIDs[detected.ID] = id
		members[detected.ID] = entityIDs
	}
	result.Communities = make([]Membership, len(hierarchy.Communities))
	for index, detected := range hierarchy.Communities {
		var parentID *CommunityID
		if detected.ParentID >= 0 {
			parent := communityIDs[detected.ParentID]
			parentID = &parent
		}
		result.Communities[index] = Membership{
			ID:           communityIDs[detected.ID],
			Number:       detected.ID,
			Level:        detected.Level,
			ParentID:     parentID,
			Final:        detected.Final,
			Unsplittable: detected.Unsplittable,
			EntityIDs:    members[detected.ID],
		}
	}
	sort.Slice(result.Communities, func(left, right int) bool {
		if result.Communities[left].Level != result.Communities[right].Level {
			return result.Communities[left].Level < result.Communities[right].Level
		}
		return result.Communities[left].Number < result.Communities[right].Number
	})
	if err := ValidateCommunitySet(result); err != nil {
		return CommunitySet{}, err
	}
	return result, nil
}
