package journal

// ActionSignals provides best-effort in-memory wakeups. Persistent Journal
// state remains the only recovery source, so a full channel is safe to coalesce.
type ActionSignals struct {
	wakeups chan struct{}
}

func NewActionSignals() *ActionSignals {
	return &ActionSignals{
		wakeups: make(chan struct{}, 1),
	}
}

func (signals *ActionSignals) Wakeups() <-chan struct{} {
	if signals == nil {
		return nil
	}
	return signals.wakeups
}

func (signals *ActionSignals) notify() {
	if signals == nil {
		return
	}
	select {
	case signals.wakeups <- struct{}{}:
	default:
	}
}
