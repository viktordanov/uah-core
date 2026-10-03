package coordinator

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/viktordanov/uah-core/harness/contextbuilder"
	"github.com/viktordanov/uah-core/harness/llm"
	"github.com/viktordanov/uah-core/harness/session"
	"github.com/viktordanov/uah-core/harness/sessionstore"
	"github.com/viktordanov/uah-core/harness/tool"
)

func effortUpdate(effort llm.ReasoningEffort) llm.Item {
	return llm.Item{Type: llm.ItemConfigurationUpdate, Data: llm.ConfigurationUpdate{ReasoningEffort: effort}}
}

func TestCoordinatorRecordsEffortUpdateWithTurn(t *testing.T) {
	store := emptyFakeStore()
	inputs := newTestInbox(t)
	started := make(chan llm.Request, 1)
	adapter := &fakeAdapter{respond: func(ctx context.Context, request llm.Request) (llm.Response, error) {
		started <- request
		<-ctx.Done()
		return llm.Response{}, ctx.Err()
	}}
	builder := contextbuilder.NewBuilder()
	builder.SetModel(llm.Model{ID: "model", ReasoningEffort: llm.ReasoningEffortHigh})
	current := newTestCoordinatorWithAdapter(store, inputs, newFakeOperationManager(), builder, tool.NewRegistry(tool.StaticTranslators{}), adapter)
	var asked []llm.Request
	current.dependencies.EffortUpdate = func(request llm.Request) llm.ReasoningEffort {
		asked = append(asked, request)
		return llm.ReasoningEffortLow
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- current.Run(ctx) }()

	submitTestInput(t, inputs, externalEvent(t, 1, "input-1", "hello"))
	request := receiveTestValue(t, started)
	user := llm.Item{Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleUser, Text: "hello"}}
	if want := withPreamble(t, user, effortUpdate(llm.ReasoningEffortLow)); !reflect.DeepEqual(request.Input, want) {
		t.Fatalf("model input = %#v, want %#v", request.Input, want)
	}
	if request.Model.ReasoningEffort != llm.ReasoningEffortHigh {
		t.Fatalf("request effort = %q, want the builder's", request.Model.ReasoningEffort)
	}
	if len(asked) != 1 || !reflect.DeepEqual(asked[0].Input, withPreamble(t, user)) {
		t.Fatalf("EffortUpdate saw %#v, want the request without the update", asked)
	}
	if len(store.appendedTurns) != 1 || store.appendedTurns[0].EffortUpdate != llm.ReasoningEffortLow {
		t.Fatalf("appended turns = %#v", store.appendedTurns)
	}

	cancel()
	if err := receiveTestValue(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context cancellation", err)
	}
}

func TestCoordinatorRejectsInvalidEffortUpdate(t *testing.T) {
	store := emptyFakeStore()
	inputs := newTestInbox(t)
	current := newTestCoordinator(store, inputs, newFakeOperationManager(), contextbuilder.NewBuilder(), tool.NewRegistry(tool.StaticTranslators{}))
	current.dependencies.EffortUpdate = func(llm.Request) llm.ReasoningEffort { return "ultra" }
	done := make(chan error, 1)
	go func() { done <- current.Run(t.Context()) }()

	submitTestInput(t, inputs, externalEvent(t, 1, "input-1", "hello"))
	if err := receiveTestValue(t, done); err == nil || !strings.Contains(err.Error(), `effort update "ultra"`) {
		t.Fatalf("Run error = %v, want the invalid effort", err)
	}
	if len(store.appendedTurns) != 0 {
		t.Fatalf("appended turns = %#v, want none", store.appendedTurns)
	}
}

func TestCoordinatorRestoresEffortUpdate(t *testing.T) {
	user := externalEvent(t, 1, "input-1", "hello")
	turn := session.Turn{ID: "turn-1", Type: session.TurnRegular, EffortUpdate: llm.ReasoningEffortLow}
	response := llm.Response{Output: []llm.Item{
		{Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleAssistant, Text: "done"}},
	}}
	store := &fakeStore{
		resume: emptyFakeStore().resume,
		items: []sessionstore.Item{
			storedItem(1, sessionstore.ItemInput, user),
			storedItem(2, sessionstore.ItemTurn, turn),
			storedItem(3, sessionstore.ItemModelResponse, sessionstore.ModelResponse{TurnID: turn.ID, Response: response}),
		},
	}
	builder := contextbuilder.NewBuilder()
	current := newTestCoordinator(store, newTestInbox(t), newFakeOperationManager(), builder, tool.NewRegistry(tool.StaticTranslators{}))
	if err := current.restore(t.Context()); err != nil {
		t.Fatal(err)
	}

	built, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	want := withPreamble(t,
		llm.Item{Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleUser, Text: "hello"}},
		effortUpdate(llm.ReasoningEffortLow),
		response.Output[0],
	)
	if !reflect.DeepEqual(built.Request.Input, want) {
		t.Fatalf("restored input = %#v, want %#v", built.Request.Input, want)
	}
}
