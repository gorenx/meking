package knowledge_test

import (
	"context"
	"testing"

	"github.com/memoria-space/meking/zone"
)

func testZoneContext(t *testing.T) context.Context {
	t.Helper()
	ctx, err := zone.NewContext(t.Context(), "10000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}
