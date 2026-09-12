package text

import (
	"strings"
	"testing"

	"github.com/memoria-space/meking/corpus/document"
)

func TestValidateRejectsInvalidBody(t *testing.T) {
	documentValue, err := document.New("source.txt", "text/plain", []byte("source"))
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		" ",
		string([]byte{0xff}),
		"before\x00after",
		strings.Repeat("a", MaximumBodyBytes+1),
	} {
		err := Validate(Text{
			ID: "8e48173a-5b52-455a-9f4c-24851a95a969",
			DocumentID: documentValue.ID, Title: "Source", Body: body, Format: PlainText,
			ExtractionProfile: "plain/v1", NormalizationProfile: "standard/v1",
		})
		if err == nil {
			t.Fatalf("Validate() accepted invalid body of %d bytes", len(body))
		}
	}
}
