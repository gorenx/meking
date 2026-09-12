package knowledge

import "encoding/hex"

type EntityID string
type RelationID string
type ClaimID string

// Version identifies one immutable formal value inside a single Knowledge ID.
// Zero is reserved for the initial-conflict base and is never persisted.
type Version uint64

type Hash [32]byte

func (hash Hash) String() string {
	return hex.EncodeToString(hash[:])
}

type Entity struct {
	ID          EntityID `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	Aliases     []string `json:"aliases"`
	Description string   `json:"description"`
}

type Relation struct {
	ID             RelationID `json:"id"`
	SourceEntityID EntityID   `json:"source_entity_id"`
	TargetEntityID EntityID   `json:"target_entity_id"`
	Type           string     `json:"type"`
	Description    string     `json:"description"`
}

type Subject interface {
	ObjectRef
	claimSubject()
}

func NewEntitySubject(id EntityID) (EntityID, error) {
	if err := ValidateEntityID(id); err != nil {
		return "", err
	}
	return id, nil
}

func NewRelationSubject(id RelationID) (RelationID, error) {
	if err := ValidateRelationID(id); err != nil {
		return "", err
	}
	return id, nil
}

type Claim struct {
	ID          ClaimID
	Subject     Subject
	Type        string
	Description string
}

type Knowledge interface {
	Entity | Relation | Claim
}

type KnowledgeID interface {
	EntityID | RelationID | ClaimID
}

// KnowledgeVersion is one immutable formal value. Deleted Versions retain the
// preceding content and Hash so exact history remains complete.
type KnowledgeVersion[T Knowledge] struct {
	Knowledge T
	Version   Version
	Deleted   bool
	Hash      Hash
}

type ObjectRef interface {
	ObjectID() string
	objectType() string
}

func (id EntityID) ObjectID() string { return string(id) }
func (EntityID) objectType() string  { return "entity" }
func (EntityID) claimSubject()       {}

func (id RelationID) ObjectID() string { return string(id) }
func (RelationID) objectType() string  { return "relation" }
func (RelationID) claimSubject()       {}

func (id ClaimID) ObjectID() string { return string(id) }
func (ClaimID) objectType() string  { return "claim" }

type Reference[ID KnowledgeID] struct {
	ID      ID      `json:"id"`
	Version Version `json:"version"`
}

type Page[T any, Cursor comparable] struct {
	Items     []T
	NextAfter Cursor
	HasMore   bool
}
