// Package contextbuilder defines I/O-pure, in-memory model request construction.
package contextbuilder

import (
	"github.com/viktordanov/uah-core/harness/inbox"
	"github.com/viktordanov/uah-core/harness/llm"
)

type ChangeKind string

const (
	ChangeOmitted   ChangeKind = "omitted"
	ChangeTruncated ChangeKind = "truncated"
	ChangeCompacted ChangeKind = "compacted"
)

type Change struct {
	Kind   ChangeKind
	Source string
	Reason string
}

type Report struct {
	Changes []Change
}

type Result struct {
	Request llm.Request
	Report  Report
}

// Builder retains model request state without performing I/O.
type Builder interface {
	// AddExternalInput adds an InputExternal input as a user message and an
	// InputDeveloper input as a developer message.
	AddExternalInput(inbox.Input) error
	AddControlMessage(inbox.ControlMessage)
	SetModel(llm.Model)
	SetSystemPrompt(string)
	AddModelResponse(llm.Response)
	AddReasoning(llm.Reasoning)
	AddTool(llm.Tool)
	AddToolResult(string, []llm.ToolResultOutput, bool)
	// AddConfigurationUpdate adds a configuration update that sets the
	// effort from here on, leaving the request's effort as it is.
	AddConfigurationUpdate(llm.ReasoningEffort)
	Commit()
	Build() (Result, error)
}
