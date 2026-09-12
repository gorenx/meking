package provenance_test

import (
	"testing"

	"github.com/memoria-space/meking/knowledge/provenance"
)

func TestValidateSourceDoesNotOwnCorpora(t *testing.T) {
	valid := provenance.Source{ID: "source/extraction/event-1", Kind: provenance.Extraction, ProducerID: "event-1"}
	if err := provenance.Validate(valid); err != nil {
		t.Fatal(err)
	}
	for _, source := range []provenance.Source{
		{Kind: provenance.Extraction, ProducerID: "event-1"},
		{ID: "source-1", Kind: "other", ProducerID: "event-1"},
		{ID: "source-1", Kind: provenance.Agent},
	} {
		if err := provenance.Validate(source); err == nil {
			t.Fatalf("Validate(%#v) error = nil", source)
		}
	}
}
