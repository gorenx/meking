package community

import "errors"

var (
	// ErrEmptyGraph is returned when no relationship edge is available.
	ErrEmptyGraph = errors.New("community: graph has no edges")

	// ErrInvalidMaxClusterSize is returned for a non-positive hierarchy limit.
	ErrInvalidMaxClusterSize = errors.New("community: max cluster size must be positive")

	// ErrInvalidHierarchy is returned when generated memberships violate tree invariants.
	ErrInvalidHierarchy = errors.New("community: invalid hierarchy")
)

// DetectConfig controls hierarchical clustering used to build report communities.
type DetectConfig struct {
	// MaxClusterSize triggers another Leiden run for communities at or above
	// this size. A community that cannot split is retained and marked.
	MaxClusterSize int

	// UseLargestConnectedComponent removes every component except the largest
	// while preserving the exact Entity IDs supplied by Graph.
	UseLargestConnectedComponent bool

	// Seed initializes the deterministic random stream shared by all levels.
	Seed int64
}

// Community describes one node set at one level of the hierarchy.
type Community struct {
	// ID is unique across all levels of one hierarchy.
	ID int

	// Level is zero for root communities and increases for recursive splits.
	Level int

	// ParentID is -1 for roots and otherwise references exactly one parent.
	ParentID int

	// Nodes contains stable Entity IDs in deterministic algorithm order.
	Nodes []string

	// Final reports whether Nodes is not superseded by finer child communities.
	Final bool

	// Unsplittable reports that the community reached the size threshold but
	// another Leiden run returned a single community.
	Unsplittable bool
}

// Hierarchy is the validated result of one community-detection run.
type Hierarchy struct {
	// Communities is ordered by level and then by community ID.
	Communities []Community
}

// ReportFinding is one model-generated insight grounded in community context.
type ReportFinding struct {
	// Summary is the concise heading rendered for the insight.
	Summary string `json:"summary"`

	// Explanation provides the detailed evidence and data references for the insight.
	Explanation string `json:"explanation"`
}

// ReportDraft is the structured model response before community metadata is attached.
type ReportDraft struct {
	// Title names the community using its representative entities.
	Title string `json:"title"`

	// Summary describes the community's overall structure and important information.
	Summary string `json:"summary"`

	// Findings contains the detailed insights used as report subsections.
	Findings []ReportFinding `json:"findings"`

	// Rating scores the community's impact severity as requested by the prompt.
	Rating float64 `json:"rating"`

	// RatingExplanation explains the impact rating in one sentence.
	RatingExplanation string `json:"rating_explanation"`
}

// ReportModelRequest carries the rendered prompt required by the report use case.
type ReportModelRequest struct {
	// Prompt contains the complete graph context and report instructions.
	Prompt string
}

// ReportFragmentRequest carries one rendered evidence batch or intermediate
// merge prompt. It is distinct from ReportModelRequest because its response is
// not a publishable Community Report and has no title or impact rating.
type ReportFragmentRequest struct {
	Prompt string
}

// ReportFragment is a grounded intermediate summary used only while reducing
// an over-budget Community input. Generator preserves its order and requires a
// non-empty Content value before it can participate in another merge round.
type ReportFragment struct {
	Content string `json:"content"`
}
