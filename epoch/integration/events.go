// Package integration defines Epoch-owned publication facts.
package integration

import (
	"fmt"
	"strings"

	"github.com/memoria-space/meking/internal/uuid"
	"github.com/memoria-space/meking/knowledge"
)

type (
	EpochID     int64
	CorporaID   string
	StructureID string
)

func Stream(structureID string) string {
	return fmt.Sprintf("epoch/%s", structureID)
}

func ValidateStream(streamID string) error {
	const prefix = "epoch/"
	structureID := strings.TrimPrefix(streamID, prefix)
	if structureID == streamID || !uuid.IsCanonicalV4(structureID) {
		return fmt.Errorf("invalid Epoch StreamID")
	}
	return nil
}

type PublishedV1 struct {
	EpochID     EpochID              `json:"epoch_id"`
	CorporaID   CorporaID            `json:"corpora_id"`
	Knowledge   knowledge.Manifest `json:"knowledge"`
	StructureID StructureID          `json:"structure_id"`
}

type Body interface {
	epochEvent()
	EventType() string
	SchemaVersion() uint32
}

func (PublishedV1) epochEvent() {}

func (PublishedV1) EventType() string { return "epoch.published" }

func (PublishedV1) SchemaVersion() uint32 { return 1 }

func (event PublishedV1) Validate() error {
	if string(event.CorporaID) == "" ||
		!uuid.IsCanonicalV4(string(event.StructureID)) ||
		event.EpochID <= 0 {
		return fmt.Errorf("invalid published Epoch")
	}
	if err := event.Knowledge.Validate(); err != nil {
		return fmt.Errorf("invalid published Epoch: %w", err)
	}
	return nil
}
