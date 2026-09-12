package drift

type requestBudget struct {
	limits       RequestLimits
	usage        Usage
	heldCalls    int
	heldOutput   int
	activeOutput int
}

func newRequestBudget(limits RequestLimits, usage Usage) *requestBudget {
	return &requestBudget{limits: limits, usage: usage}
}

func (b *requestBudget) reserve(promptTokens int, completionTokens int) bool {
	if !b.canReserve(promptTokens, completionTokens) {
		return false
	}
	b.usage.Calls++
	b.usage.PromptTokens += promptTokens
	b.activeOutput += completionTokens
	return true
}

func (b *requestBudget) finish(completionTokens int, actualOutput int) bool {
	b.activeOutput -= completionTokens
	if b.activeOutput < 0 {
		b.activeOutput = 0
	}
	b.usage.OutputTokens += actualOutput
	return actualOutput <= completionTokens
}

func (b *requestBudget) hold(calls int, outputTokens int) bool {
	switch {
	case b.usage.Calls+b.heldCalls+calls > b.limits.ModelCalls:
		return false
	case b.usage.OutputTokens+b.heldOutput+b.activeOutput+outputTokens > b.limits.OutputTokens:
		return false
	default:
		b.heldCalls += calls
		b.heldOutput += outputTokens
		return true
	}
}

func (b *requestBudget) useHeld(promptTokens int, completionTokens int) bool {
	switch {
	case b.heldCalls < 1:
		return false
	case b.heldOutput < completionTokens:
		return false
	case b.usage.PromptTokens+promptTokens > b.limits.PromptTokens:
		return false
	case b.usage.OutputTokens+b.heldOutput+b.activeOutput > b.limits.OutputTokens:
		return false
	}
	b.heldCalls--
	b.heldOutput -= completionTokens
	b.usage.Calls++
	b.usage.PromptTokens += promptTokens
	b.activeOutput += completionTokens
	return true
}

func (b *requestBudget) capacity(completionTokens int) int {
	if b.usage.PromptTokens >= b.limits.PromptTokens {
		return 0
	}
	calls := b.limits.ModelCalls - b.usage.Calls - b.heldCalls
	output := (b.limits.OutputTokens - b.usage.OutputTokens - b.heldOutput - b.activeOutput) / completionTokens
	if calls <= 0 || output <= 0 {
		return 0
	}
	return min(calls, output)
}

func (b *requestBudget) remainingPromptTokens() int {
	return max(0, b.limits.PromptTokens-b.usage.PromptTokens)
}

func (b *requestBudget) canReserve(promptTokens int, completionTokens int) bool {
	return b.usage.Calls+b.heldCalls+1 <= b.limits.ModelCalls &&
		b.usage.PromptTokens+promptTokens <= b.limits.PromptTokens &&
		b.usage.OutputTokens+b.heldOutput+b.activeOutput+completionTokens <= b.limits.OutputTokens
}
