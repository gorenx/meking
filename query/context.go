package query

// ConversationRole identifies one supported conversation turn kind shared by
// query use cases that accept prior user and assistant exchanges.
type ConversationRole string

const (
	RoleSystem    ConversationRole = "system"
	RoleUser      ConversationRole = "user"
	RoleAssistant ConversationRole = "assistant"
)

// ConversationTurn is one immutable prompt-history entry supplied by a query
// caller. Validation belongs to the consuming query because supported ordering
// and history limits are use-case rules.
type ConversationTurn struct {
	Role    ConversationRole
	Content string
}

// ContextRow records one rendered evidence row and whether it was admitted to
// the model context after token-budget evaluation.
type ContextRow struct {
	Values    []string
	InContext bool
}

// ContextSection records the ordered rows considered for one rendered context
// table. TracksInContext is false for diagnostic-only sections whose rows were
// not independently budgeted.
type ContextSection struct {
	Name            string
	Columns         []string
	Rows            []ContextRow
	TracksInContext bool
}
