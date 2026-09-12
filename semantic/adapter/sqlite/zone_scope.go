package sqlite

import (
	"context"
	"fmt"

	"github.com/memoria-space/meking/zone"
)

const zoneIDFilterKey = "zone_id"

func semanticZoneID(ctx context.Context) (string, error) {
	zoneID, err := zone.RequireID(ctx)
	if err != nil {
		return "", err
	}
	return string(zoneID), nil
}

func validateSemanticSearchFilter(zoneID string, requested map[string]string) error {
	if zoneID == "" {
		return fmt.Errorf("Semantic Namespace %q is missing", zoneIDFilterKey)
	}
	for key, value := range requested {
		if key != zoneIDFilterKey {
			return fmt.Errorf("Semantic search Filter %q is not supported", key)
		}
		if value != zoneID {
			return fmt.Errorf("Semantic search Filter %q does not match Namespace Zone", key)
		}
	}
	return nil
}
