package llm

import (
	"reflect"
	"testing"
)

func update(effort ReasoningEffort) Item {
	return Item{Type: ItemConfigurationUpdate, Data: ConfigurationUpdate{ReasoningEffort: effort}}
}

func TestRequestEffort(t *testing.T) {
	user := Item{Type: ItemMessage, Data: Message{Role: RoleUser, Text: "hi"}}
	request := Request{Model: Model{ID: "m", ReasoningEffort: ReasoningEffortHigh}, Input: []Item{user}}
	if got := request.Effort(); got != ReasoningEffortHigh {
		t.Fatalf("Effort() without updates = %q, want the request's", got)
	}
	request.Input = []Item{user, update(ReasoningEffortLow), user, update(ReasoningEffortMedium), user}
	if got := request.Effort(); got != ReasoningEffortMedium {
		t.Fatalf("Effort() = %q, want the last update's", got)
	}
}

func TestRequestWithoutConfigurationUpdates(t *testing.T) {
	user := Item{Type: ItemMessage, Data: Message{Role: RoleUser, Text: "hi"}}
	request := Request{
		Model: Model{ID: "m", ReasoningEffort: ReasoningEffortHigh},
		Input: []Item{user, update(ReasoningEffortLow), user},
	}
	got := request.WithoutConfigurationUpdates()
	want := Request{Model: Model{ID: "m", ReasoningEffort: ReasoningEffortHigh}, Input: []Item{user, user}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("WithoutConfigurationUpdates() = %#v, want %#v", got, want)
	}
	if request.Input[1].Type != ItemConfigurationUpdate {
		t.Fatal("WithoutConfigurationUpdates changed the request's input")
	}

	plain := Request{Model: Model{ReasoningEffort: ReasoningEffortHigh}, Input: []Item{user}}
	if got := plain.WithoutConfigurationUpdates(); !reflect.DeepEqual(got, plain) {
		t.Fatalf("WithoutConfigurationUpdates() without updates = %#v, want the request", got)
	}
}
