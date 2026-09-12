// Package deletion owns the explicit Knowledge deletion entry. Missing content
// in a submission and empty descriptions never invoke this application.
package deletion

import (
	"context"
	"errors"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/provenance"
	"github.com/memoria-space/meking/transaction"
)

var ErrKnowledgeInUse = errors.New("Knowledge is in use")

type Command struct {
	Source          provenance.Source
	Knowledge       knowledge.ObjectRef
	ExpectedVersion knowledge.Version
}

type Result struct {
	Version        knowledge.Version
	CreatedVersion bool
}

type KnowledgeReferences interface {
	ActiveRelations(context.Context, knowledge.EntityID) ([]knowledge.RelationID, error)
	ActiveClaims(context.Context, knowledge.Subject) ([]knowledge.ClaimID, error)
}

type Provenance interface {
	Source(context.Context, string) (provenance.Source, bool, error)
	RecordSource(context.Context, provenance.Source, []provenance.Evidence) error
	DeletionResult(context.Context, string) (Result, error)

	ConfirmEntity(context.Context, provenance.EntityConfirmation) error
	ConfirmRelation(context.Context, provenance.RelationConfirmation) error
	ConfirmClaim(context.Context, provenance.ClaimConfirmation) error
}

type Dependencies struct {
	Tx                  transaction.Tx
	VersionHistory      knowledge.VersionHistory
	KnowledgeReferences KnowledgeReferences
	Provenance          Provenance
}

type Application struct {
	Dependencies
}
