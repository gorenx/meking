// Package transaction defines the database-independent transaction boundary
// shared by application services that must commit several module writes atomically.
package transaction

import "context"

// Tx executes one callback in a transaction. The callback context carries the
// active transaction and must be passed unchanged to every participating Store
// and Publisher call.
type Tx interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}
