package coordinator

import (
	"time"

	"github.com/viktordanov/uah-core/harness/llm"
)

// callHold is how long a call is held, and whether that is its own hold
// (CallHold) rather than Hold.
func (p WakePolicy) callHold(call llm.ToolCall) (time.Duration, bool) {
	if p.CallHold != nil {
		if hold := p.CallHold(call); hold > 0 {
			return hold, true
		}
	}

	return p.Hold, false
}
