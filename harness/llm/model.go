package llm

import "encoding/json/jsontext"

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	// RoleDeveloper marks instructions from the harness rather than the user,
	// such as the context it prepares for a new session.
	RoleDeveloper Role = "developer"
)

type ItemType string

const (
	ItemMessage    ItemType = "message"
	ItemToolCall   ItemType = "tool_call"
	ItemToolResult ItemType = "tool_result"
	ItemReasoning  ItemType = "reasoning"
)

type Item struct {
	ProviderID string
	Type       ItemType
	Data       any
}

type Message struct {
	Role  Role
	Text  string
	Phase string
}

// Custom marks a call to a custom tool. Its Arguments hold the raw input text
// the model wrote rather than a JSON object.
type ToolCall struct {
	CallID    string
	Name      string
	Arguments string
	Custom    bool `json:",omitzero"`
}

type ToolResultKind string

const (
	ToolResultText  ToolResultKind = "text"
	ToolResultImage ToolResultKind = "image"
)

type ToolResultOutput struct {
	Kind  ToolResultKind
	Value string
}

type ToolResult struct {
	CallID string
	Output []ToolResultOutput
}

// Raw is the provider's verbatim reasoning item. A provider may attach state to
// it that the harness cannot reconstruct, such as encrypted reasoning content, so
// adapters replay Raw unchanged instead of re-encoding Summary.
type Reasoning struct {
	Summary []string       `json:",omitzero"`
	Raw     jsontext.Value `json:",omitzero"`
}

type ToolType string

const (
	ToolFunction ToolType = "function"
	ToolHosted   ToolType = "hosted"
	// ToolCustom takes free-form text input instead of JSON arguments. Its
	// calls arrive with ToolCall.Custom set.
	ToolCustom ToolType = "custom"
)

type Tool struct {
	Type        ToolType
	Name        string
	Description string
	Parameters  map[string]any
	// Grammar constrains a custom tool's input. Nil leaves it unconstrained text.
	Grammar *ToolGrammar `json:",omitzero"`
}

// ToolGrammar is a grammar the provider samples a custom tool's input from.
type ToolGrammar struct {
	// Syntax is "lark" or "regex".
	Syntax     string
	Definition string
}

type Model struct {
	ID              string
	MaxOutputTokens *int64
	ReasoningEffort ReasoningEffort
}

type ReasoningEffort string

const (
	ReasoningEffortLow    ReasoningEffort = "low"
	ReasoningEffortMedium ReasoningEffort = "medium"
	ReasoningEffortHigh   ReasoningEffort = "high"
	ReasoningEffortXHigh  ReasoningEffort = "xhigh"
	ReasoningEffortMax    ReasoningEffort = "max"
)

func (effort ReasoningEffort) Valid() bool {
	switch effort {
	case ReasoningEffortLow, ReasoningEffortMedium, ReasoningEffortHigh, ReasoningEffortXHigh, ReasoningEffortMax:
		return true
	default:
		return false
	}
}

type Request struct {
	Model Model
	Input []Item
	Tools []Tool
}

type StopReason string

const (
	StopComplete        StopReason = "complete"
	StopMaxOutputTokens StopReason = "max_output_tokens"
	StopRefused         StopReason = "refused"
)

type Response struct {
	ID      string
	Stop    StopReason
	Output  []Item `json:",omitzero"`
	Usage   Usage
	Failure *Failure
}

// InputTokens includes CachedInputTokens and CacheWriteInputTokens.
// OutputTokens includes ReasoningTokens.
type Usage struct {
	InputTokens           int64
	CachedInputTokens     int64
	CacheWriteInputTokens int64
	OutputTokens          int64
	ReasoningTokens       int64
	Raw                   jsontext.Value `json:",omitzero"`
}

type Failure struct {
	Code    string
	Message string
}
