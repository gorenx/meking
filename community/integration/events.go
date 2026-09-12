// Package integration defines Community-owned result event bodies.
package integration

import (
	"fmt"
	"strings"

	"github.com/memoria-space/meking/internal/uuid"
	"github.com/memoria-space/meking/knowledge"
)

type (
	CommunitySetID string
	CorporaID      string
)

type StructurePreparedV1 struct {
	StructureID    string               `json:"structure_id"`
	CommunitySetID CommunitySetID       `json:"community_set_id"`
	CorporaID      CorporaID            `json:"corpora_id"`
	Knowledge      knowledge.Manifest `json:"knowledge"`
}

type Body interface {
	communityEvent()
	EventType() string
	SchemaVersion() uint32
}

func (StructurePreparedV1) communityEvent() {}

func (StructurePreparedV1) EventType() string { return "community.structure_prepared" }

func (StructurePreparedV1) SchemaVersion() uint32 { return 1 }

func (event StructurePreparedV1) Validate() error {
	if !uuid.IsCanonicalV4(event.StructureID) ||
		!uuid.IsCanonicalV4(string(event.CommunitySetID)) ||
		strings.TrimSpace(string(event.CorporaID)) == "" {
		return fmt.Errorf("invalid prepared Community Structure")
	}
	if err := event.Knowledge.Validate(); err != nil {
		return fmt.Errorf("invalid prepared Community Structure: %w", err)
	}
	return nil
}

func StructureStream(structureID string) string {
	return "community-structure/" + structureID
}

func ValidateStructureStream(streamID string) error {
	const prefix = "community-structure/"
	value := strings.TrimPrefix(streamID, prefix)
	if value == streamID || !uuid.IsCanonicalV4(value) {
		return fmt.Errorf("invalid Community Structure StreamID")
	}
	return nil
}
