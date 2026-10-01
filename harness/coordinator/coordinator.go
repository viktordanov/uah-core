// Package coordinator defines the owner of the central event loop.
package coordinator

import (
	"context"
	"time"

	"github.com/viktordanov/unreal-agent/harness/contextbuilder"
	"github.com/viktordanov/unreal-agent/harness/inbox"
	"github.com/viktordanov/unreal-agent/harness/llm"
	"github.com/viktordanov/unreal-agent/harness/operation"
	"github.com/viktordanov/unreal-agent/harness/session"
	"github.com/viktordanov/unreal-agent/harness/sessionstore"
	"github.com/viktordanov/unreal-agent/harness/tool"
)

type Dependencies struct {
	ToolHeartbeatInterval time.Duration
	SessionID             session.ID
	Inbox                 *inbox.Inbox
	Restored              sessionstore.ResumeState
	Sessions              sessionstore.Store
	ContextBuilder        contextbuilder.Builder
	LLM                   llm.Adapter
	Tools                 tool.Registry
	Operations            operation.Manager
	// Wake tunes when finished tool calls wake the model. Its zero value
	// keeps the default: after the grace period, each result wakes it.
	Wake WakePolicy
}

// WakePolicy holds experimental rules for when tool results wake the model.
// Each rule is off at its zero value, and the rules combine. An inbox input,
// such as a user message or a heartbeat, always wakes the model.
type WakePolicy struct {
	// Batch holds a turn's results until every call the turn issued has
	// finished, so the model never sees a placeholder for its latest calls.
	Batch bool
	// Debounce holds a result that lands while other calls still run, for at
	// most this long, so results that land close together arrive in one turn.
	Debounce time.Duration
	// AllDone has a turn that issues no tool calls while calls run sleep until
	// every running call has finished or one has failed.
	AllDone bool
	// Yield returns how long a turn waits for a call before the call
	// continues in the background and the model wakes with its output so
	// far; zero keeps the grace period. A turn with a yielding call also
	// holds its immediate results until the wait ends.
	Yield func(llm.ToolCall) time.Duration
	// Progress renders the output so far of a call that outlived its yield;
	// nil or an empty result shows no output.
	Progress func([]operation.Operation) string
}

// holdsTurn reports whether the policy holds a turn's results past the
// grace period.
func (policy WakePolicy) holdsTurn() bool {
	return policy.Batch || policy.Yield != nil
}

type Coordinator interface {
	// Run owns one session's decision loop until a stop control completes or
	// the context is canceled. It returns nil for a completed stop. It is
	// single-use; its caller must cancel the Inbox when Run returns.
	Run(context.Context) error
}

func New(dependencies Dependencies) Coordinator {
	return &coordinator{
		dependencies: dependencies,
		state:        newLoopState(),
	}
}
