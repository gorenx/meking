package journal

import (
	"context"
)

type ZoneMerger interface {
	MergeChildTexts(ctx context.Context) error
	MergeTextUnits(ctx context.Context) error
}
