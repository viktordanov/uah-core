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

// WakePolicy holds a turn's results back so that the model is not woken
// just to hear that a call is still running. Its zero value keeps the
// default grace period. An inbox input, such as a user message or a
// heartbeat, always wakes the model.
type WakePolicy struct {
	// Hold, when set, holds a turn's results, its immediate ones included,
	// until every call the turn issued has finished. A call still running
	// after Hold, from any turn, wakes the model with its output so far and
	// stops holding the turn back.
	Hold time.Duration
	// Progress renders the output so far of a call that outlived Hold; nil
	// or an empty result shows no output.
	Progress func([]operation.Operation) string
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
