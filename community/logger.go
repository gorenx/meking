package community

import (
	"context"
	"log/slog"
)

// CommunityLogger records domain result identities and counts without
// knowledge or report content.
type CommunityLogger interface {
	LogAttrs(
		ctx context.Context,
		level slog.Level,
		message string,
		attributes ...slog.Attr,
	)
}
