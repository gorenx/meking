package knowledge

// EntityContent is one complete Entity value addressed by its stable business
// identity before a persistent EntityID is resolved.
type EntityContent struct {
	Identity    EntityIdentity `json:"identity"`
	Aliases     []string       `json:"aliases"`
	Description string         `json:"description"`
}

// RelationContent is one complete directed Relation addressed by the stable
// identities of both endpoint Entities.
type RelationContent struct {
	Source      EntityIdentity `json:"source"`
	Target      EntityIdentity `json:"target"`
	Type        string         `json:"type"`
	Description string         `json:"description"`
}

// SubjectIdentity is the stable Entity or Relation identity named by content
// before persistence resolves it to an EntityID or RelationID.
type SubjectIdentity interface {
	subjectIdentity()
}

func (EntityIdentity) subjectIdentity() {}

// RelationIdentity identifies a typed Relation by its endpoint identities.
type RelationIdentity struct {
	Source EntityIdentity `json:"source"`
	Target EntityIdentity `json:"target"`
	Type   string         `json:"type"`
}

func (RelationIdentity) subjectIdentity() {}

// ClaimContent is one complete parser-produced Claim before its stable ClaimID
// and concrete Subject reference are resolved.
type ClaimContent struct {
	Subject     SubjectIdentity
	Type        string
	Description string
}
