// Package integration defines Corpus-owned facts published with owner writes.
// It contains no Journal or persistence implementation types.
package integration

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/memoria-space/meking/corpus/document"
	"github.com/memoria-space/meking/corpus/text"
	"github.com/memoria-space/meking/zone"
)

type (
	DocumentID string
	TextID     string
	CorporaID  string
)

func DocumentStream(documentID DocumentID) string {
	return fmt.Sprintf("document/%s", documentID)
}

func TextStream(textID TextID) string {
	return fmt.Sprintf("text/%s", textID)
}

func ValidateTextStream(streamID string) error {
	const prefix = "text/"
	value := strings.TrimPrefix(streamID, prefix)
	if value == streamID || text.ValidateID(text.ID(value)) != nil {
		return fmt.Errorf("invalid Text StreamID")
	}
	return nil
}

func CorporaStream(corporaID CorporaID) string {
	return fmt.Sprintf("corpora/%s", corporaID)
}

func ValidateCorporaStream(streamID string) error {
	const prefix = "corpora/"
	value := strings.TrimPrefix(streamID, prefix)
	if value == streamID || !validCorporaID(CorporaID(value)) {
		return fmt.Errorf("invalid Corpora StreamID")
	}
	return nil
}

type DocumentRecordedV1 struct {
	DocumentID    DocumentID     `json:"document_id"`
	ContentDigest string         `json:"content_digest"`
	Source        DocumentSource `json:"source"`
}

type TextPreparedV1 struct {
	DocumentID DocumentID `json:"document_id"`
	TextID     TextID     `json:"text_id"`
	Source     TextSource `json:"source"`
}

type DocumentSource struct {
	ZoneID     string     `json:"zone_id"`
	DocumentID DocumentID `json:"document_id"`
}

type TextSource struct {
	ZoneID string `json:"zone_id"`
	TextID TextID `json:"text_id"`
}

type TextUnitsPreparedV1 struct {
	CorporaID         CorporaID `json:"corpora_id"`
	TextID            TextID    `json:"text_id"`
	TextUnitSetDigest string    `json:"text_unit_set_digest"`
	TextUnitCount     uint64    `json:"text_unit_count"`
	TextUnitSpanCount uint64    `json:"text_unit_span_count"`
}

type CorporaPreparedV1 struct {
	CorporaID         CorporaID `json:"corpora_id"`
	TextCount         uint64    `json:"text_count"`
	TextUnitSetDigest string    `json:"text_unit_set_digest"`
	TextUnitCount     uint64    `json:"text_unit_count"`
	TextUnitSpanCount uint64    `json:"text_unit_span_count"`
}

type Body interface {
	corpusEvent()
	EventType() string
	SchemaVersion() uint32
}

func (DocumentRecordedV1) corpusEvent()  {}
func (TextPreparedV1) corpusEvent()      {}
func (TextUnitsPreparedV1) corpusEvent() {}
func (CorporaPreparedV1) corpusEvent()   {}

func (DocumentRecordedV1) EventType() string  { return "corpus.document_recorded" }
func (TextPreparedV1) EventType() string      { return "corpus.text_prepared" }
func (TextUnitsPreparedV1) EventType() string { return "corpus.text_units_prepared" }
func (CorporaPreparedV1) EventType() string   { return "corpus.corpora_prepared" }

func (DocumentRecordedV1) SchemaVersion() uint32  { return 1 }
func (TextPreparedV1) SchemaVersion() uint32      { return 1 }
func (TextUnitsPreparedV1) SchemaVersion() uint32 { return 1 }
func (CorporaPreparedV1) SchemaVersion() uint32   { return 1 }

func (event DocumentRecordedV1) Validate() error {
	if err := document.ValidateID(document.ID(event.DocumentID)); err != nil {
		return fmt.Errorf("DocumentID: %w", err)
	}
	if len(event.ContentDigest) != 64 {
		return fmt.Errorf("ContentDigest is invalid")
	}
	if _, err := hex.DecodeString(event.ContentDigest); err != nil || strings.ToLower(event.ContentDigest) != event.ContentDigest {
		return fmt.Errorf("ContentDigest is invalid")
	}
	if _, err := zone.ParseID(event.Source.ZoneID); err != nil {
		return fmt.Errorf("Source ZoneID is invalid")
	}
	if err := document.ValidateID(document.ID(event.Source.DocumentID)); err != nil {
		return fmt.Errorf("Source DocumentID: %w", err)
	}
	return nil
}

func (event TextPreparedV1) Validate() error {
	if err := document.ValidateID(document.ID(event.DocumentID)); err != nil {
		return fmt.Errorf("DocumentID: %w", err)
	}
	if err := text.ValidateID(text.ID(event.TextID)); err != nil {
		return fmt.Errorf("TextID: %w", err)
	}
	if _, err := zone.ParseID(event.Source.ZoneID); err != nil {
		return fmt.Errorf("Source ZoneID is invalid")
	}
	if err := text.ValidateID(text.ID(event.Source.TextID)); err != nil {
		return fmt.Errorf("Source TextID: %w", err)
	}
	return nil
}

func (event TextUnitsPreparedV1) Validate() error {
	if err := text.ValidateID(text.ID(event.TextID)); err != nil {
		return fmt.Errorf("TextID: %w", err)
	}
	if !validCorporaID(event.CorporaID) || !validDigest(event.TextUnitSetDigest) ||
		event.TextUnitCount == 0 || event.TextUnitSpanCount < event.TextUnitCount {
		return fmt.Errorf("invalid prepared TextUnits")
	}
	return nil
}

func (event CorporaPreparedV1) Validate() error {
	if !validCorporaID(event.CorporaID) || event.TextCount == 0 ||
		!validDigest(event.TextUnitSetDigest) || event.TextUnitCount == 0 ||
		event.TextUnitSpanCount < event.TextUnitCount {
		return fmt.Errorf("invalid prepared Corpora")
	}
	return nil
}

func validDigest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validCorporaID(value CorporaID) bool {
	parsed, err := strconv.ParseInt(string(value), 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == string(value)
}
