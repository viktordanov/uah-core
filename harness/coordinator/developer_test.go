package coordinator

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/viktordanov/uah-core/harness/contextbuilder"
	"github.com/viktordanov/uah-core/harness/inbox"
	"github.com/viktordanov/uah-core/harness/llm"
	"github.com/viktordanov/uah-core/harness/session"
	"github.com/viktordanov/uah-core/harness/sessionstore"
	"github.com/viktordanov/uah-core/harness/tool"
)

func developerEvent(t *testing.T, id inbox.ID, text string) inbox.Input {
	t.Helper()
	input := externalEvent(t, 0, id, text)
	input.Kind = inbox.InputDeveloper
	return input
}

func TestCoordinatorRunSendsDeveloperInputWithNextUserInput(t *testing.T) {
	store := emptyFakeStore()
	inputs := newTestInbox(t)
	developer := developerEvent(t, "developer-1", "context")
	user := externalEvent(t, 1, "input-1", "hello")
	started := make(chan llm.Request, 2)
	adapter := &fakeAdapter{respond: func(ctx context.Context, request llm.Request) (llm.Response, error) {
		started <- request
		<-ctx.Done()
		return llm.Response{}, ctx.Err()
	}}
	ctx, cancel := context.WithCancel(t.Context())
	current := newTestCoordinatorWithAdapter(
		store, inputs, newFakeOperationManager(), contextbuilder.NewBuilder(),
		tool.NewRegistry(tool.StaticTranslators{}), adapter,
	)
	done := make(chan error, 1)
	go func() {
		done <- current.Run(ctx)
	}()

	submitTestInput(t, inputs, developer)
	select {
	case request := <-started:
		t.Fatalf("developer input alone requested a model response: %#v", request)
	case <-time.After(50 * slurpIdleTimeout):
	}
	submitTestInput(t, inputs, user)

	request := receiveTestValue(t, started)
	want := withPreamble(t,
		llm.Item{Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleDeveloper, Text: "context"}},
		llm.Item{Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleUser, Text: "hello"}},
	)
	if !reflect.DeepEqual(request.Input, want) {
		t.Fatalf("model input = %#v, want %#v", request.Input, want)
	}

	cancel()
	if err := receiveTestValue(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context cancellation", err)
	}
	if got := len(adapter.requestSnapshot()); got != 1 {
		t.Fatalf("model requests = %d, want 1", got)
	}
}

func TestCoordinatorRestoresDeveloperInput(t *testing.T) {
	developer := developerEvent(t, "developer-1", "context")
	user := externalEvent(t, 1, "input-1", "hello")
	turn := session.Turn{ID: "turn-1", Type: session.TurnRegular}
	response := llm.Response{Output: []llm.Item{
		{Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleAssistant, Text: "done"}},
	}}
	store := &fakeStore{
		resume: emptyFakeStore().resume,
		items: []sessionstore.Item{
			storedItem(1, sessionstore.ItemInput, developer),
			storedItem(2, sessionstore.ItemInput, user),
			storedItem(3, sessionstore.ItemTurn, turn),
			storedItem(4, sessionstore.ItemModelResponse, sessionstore.ModelResponse{TurnID: turn.ID, Response: response}),
		},
	}
	builder := contextbuilder.NewBuilder()
	current := newTestCoordinator(store, newTestInbox(t), newFakeOperationManager(), builder, tool.NewRegistry(tool.StaticTranslators{}))

	if err := current.restore(t.Context()); err != nil {
		t.Fatal(err)
	}

	if got := current.pendingInputs(); got != 0 {
		t.Fatalf("pending inputs = %d, want 0", got)
	}
	built, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	want := withPreamble(t,
		llm.Item{Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleDeveloper, Text: "context"}},
		llm.Item{Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleUser, Text: "hello"}},
		response.Output[0],
	)
	if !reflect.DeepEqual(built.Request.Input, want) {
		t.Fatalf("restored input = %#v, want %#v", built.Request.Input, want)
	}
}
