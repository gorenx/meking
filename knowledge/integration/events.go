// Package integration defines Knowledge-owned extraction and submission facts.
package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/memoria-space/meking/knowledge"
)

type CorporaID string

// PublishedV1 announces one changed formal Knowledge state. Candidate and
// provenance-only writes do not publish this event.
type PublishedV1 struct {
	CorporaID CorporaID            `json:"corpora_id"`
	Knowledge knowledge.Manifest `json:"knowledge"`
}

type EntityVectorsIndexedV1 struct {
	CorporaID CorporaID            `json:"corpora_id"`
	Knowledge knowledge.Manifest `json:"knowledge"`
	Namespace string               `json:"namespace"`
}

type Body interface {
	knowledgeEvent()
	EventType() string
	SchemaVersion() uint32
}

func (PublishedV1) knowledgeEvent()            {}
func (EntityVectorsIndexedV1) knowledgeEvent() {}

func (PublishedV1) EventType() string {
	return "knowledge.published"
}

func (EntityVectorsIndexedV1) EventType() string {
	return "knowledge.entity_vectors_indexed"
}

func (PublishedV1) SchemaVersion() uint32 {
	return 1
}

func (EntityVectorsIndexedV1) SchemaVersion() uint32 {
	return 1
}

func (event PublishedV1) Validate() error {
	if strings.TrimSpace(string(event.CorporaID)) == "" {
		return fmt.Errorf("invalid published Knowledge: CorporaID is required")
	}
	if err := event.Knowledge.Validate(); err != nil {
		return fmt.Errorf("invalid published Knowledge state: %w", err)
	}
	return nil
}

func (event EntityVectorsIndexedV1) Validate() error {
	if strings.TrimSpace(string(event.CorporaID)) == "" || !validDigest(event.Namespace) {
		return fmt.Errorf("invalid indexed Knowledge Entity vectors")
	}
	if err := event.Knowledge.Validate(); err != nil {
		return fmt.Errorf("invalid indexed Knowledge Entity vectors: %w", err)
	}
	return nil
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
