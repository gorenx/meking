package application

import (
	"context"
	"time"

	"github.com/memoria-space/meking/controlplane"
)

type PendingInput struct {
	Count uint64
	Since time.Time
}

type ActionConsumer interface {
	Pending(context.Context, controlplane.Action) (PendingInput, error)
	Start(context.Context, controlplane.Action) error
	Retry(context.Context, controlplane.Action) error
}
