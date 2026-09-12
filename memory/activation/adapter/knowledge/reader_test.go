package knowledge_test

import (
	"context"
	"errors"
	"testing"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/memory/activation"
	adapter "github.com/memoria-space/meking/memory/activation/adapter/knowledge"
)

type versions struct {
	seen           knowledge.ObjectRef
	version        knowledge.Version
	deleted, found bool
	err            error
}

func (v *versions) CurrentEntity(ctx context.Context, id knowledge.EntityID) (knowledge.KnowledgeVersion[knowledge.Entity], bool, error) {
	v.seen = id
	return knowledge.KnowledgeVersion[knowledge.Entity]{Version: v.version, Deleted: v.deleted}, v.found, v.err
}
func (v *versions) CurrentRelation(ctx context.Context, id knowledge.RelationID) (knowledge.KnowledgeVersion[knowledge.Relation], bool, error) {
	v.seen = id
	return knowledge.KnowledgeVersion[knowledge.Relation]{Version: v.version, Deleted: v.deleted}, v.found, v.err
}
func (v *versions) CurrentClaim(ctx context.Context, id knowledge.ClaimID) (knowledge.KnowledgeVersion[knowledge.Claim], bool, error) {
	v.seen = id
	return knowledge.KnowledgeVersion[knowledge.Claim]{Version: v.version, Deleted: v.deleted}, v.found, v.err
}

func TestCurrentTargetMapping(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	for _, subject := range []knowledge.ObjectRef{knowledge.EntityID(id), knowledge.RelationID(id), knowledge.ClaimID(id)} {
		for _, deleted := range []bool{false, true} {
			provider := &versions{version: 7, deleted: deleted, found: true}
			reader, err := adapter.New(provider)
			if err != nil {
				t.Fatal(err)
			}
			got, found, err := reader.ReadCurrent(t.Context(), subject)
			if err != nil || !found || got != (activation.CurrentTarget{Version: 7, Deleted: deleted}) || provider.seen != subject {
				t.Fatalf("mapping: %#v %v %v", got, found, err)
			}
			provider.found = false
			if _, found, err := reader.ReadCurrent(t.Context(), subject); err != nil || found {
				t.Fatalf("missing: %v %v", found, err)
			}
			provider.err = errors.New("read failed")
			if _, _, err := reader.ReadCurrent(t.Context(), subject); !errors.Is(err, provider.err) {
				t.Fatalf("lost provider error: %v", err)
			}
		}
	}
	provider := &versions{}
	reader, err := adapter.New(provider)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := reader.ReadCurrent(t.Context(), knowledge.EntityID("invalid")); !errors.Is(err, activation.ErrInvalidObservation) || provider.seen != nil {
		t.Fatalf("invalid identity reached provider: %v", err)
	}
	if _, err := adapter.New(nil); err == nil {
		t.Fatal("nil provider accepted")
	}
}
