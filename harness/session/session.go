// Package session defines durable session and turn identity.
package session

import (
	"time"

	"github.com/viktordanov/uah-core/harness/llm"
)

type ID string

type TurnID string

type TurnType string

const (
	TurnRegular    TurnType = "regular"
	TurnCompaction TurnType = "compaction"
)

type Session struct {
	ID        ID
	CreatedAt time.Time
}

type Turn struct {
	ID             TurnID
	PreviousTurnID TurnID
	Type           TurnType
	// EffortUpdate is the effort the turn's request set with a
	// configuration update (llm.ItemConfigurationUpdate), which goes in the
	// history after the inputs the request answers; empty when it set none.
	EffortUpdate llm.ReasoningEffort `json:",omitzero"`
}
