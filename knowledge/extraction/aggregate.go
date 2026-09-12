package extraction

import "github.com/memoria-space/meking/knowledge/submission"

type Graph struct {
	Entities  []submission.Entity
	Relations []submission.Relation
}

// Result is the independent Knowledge parsed from one TextUnit.
type Result struct {
	Entities  []submission.Entity
	Relations []submission.Relation
	Claims    []submission.Claim
}
