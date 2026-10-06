package coordinator

import (
	"context"
	"reflect"
	"testing"
	"testing/synctest"

	"github.com/viktordanov/uah-core/harness/inbox"
	"github.com/viktordanov/uah-core/harness/llm"
	"github.com/viktordanov/uah-core/harness/operation"
	"github.com/viktordanov/uah-core/harness/sessionstore"
)

func TestCoordinatorResumeProcessesQueuedHardStopBeforeModelRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newStopTestRun(t, 0)
		run.store.items = []sessionstore.Item{
			storedItem(1, sessionstore.ItemInput, externalEvent(t, 0, "pending", "hello")),
		}
		submitTestInput(t, run.inputs, stopInput(t, "stop", inbox.StopHard))
		run.start(t)
		if err := <-run.done; err != nil {
			t.Fatal(err)
		}
		if len(run.store.appendedTurns) != 0 {
			t.Fatal("resume started a model turn before accepting the queued hard stop")
		}
	})
}

func TestCoordinatorResumeProcessesQueuedSettingsBeforeModelRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newStopTestRun(t, 0)
		run.store.items = []sessionstore.Item{
			storedItem(1, sessionstore.ItemInput, externalEvent(t, 0, "pending", "hello")),
		}
		run.current.dependencies.ContextBuilder.SetModel(llm.Model{ID: "m", ReasoningEffort: llm.ReasoningEffortLow})
		adapter := &fakeAdapter{respond: func(context.Context, llm.Request) (llm.Response, error) {
			return textResponse("Hello."), nil
		}}
		run.current.dependencies.LLM = adapter
		submitTestInput(t, run.inputs, settingsInput(t, "settings", inbox.Settings{ReasoningEffort: llm.ReasoningEffortHigh}))
		submitTestInput(t, run.inputs, stopInput(t, "stop", inbox.StopWhenIdle))
		run.start(t)
		if err := <-run.done; err != nil {
			t.Fatal(err)
		}
		requests := adapter.requestSnapshot()
		if len(requests) != 1 {
			t.Fatalf("requests = %d, want one resumed request", len(requests))
		}
		if got := requests[0].Model.ReasoningEffort; got != llm.ReasoningEffortHigh {
			t.Fatalf("resumed request model = %q, want the queued model %q", got, "next")
		}
	})
}

func TestCoordinatorResumeProcessesQueuedInputBeforeModelRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newStopTestRun(t, 0)
		run.store.items = []sessionstore.Item{
			storedItem(1, sessionstore.ItemInput, externalEvent(t, 0, "pending", "hello")),
		}
		adapter := &fakeAdapter{respond: func(context.Context, llm.Request) (llm.Response, error) {
			return textResponse("Hello."), nil
		}}
		run.current.dependencies.LLM = adapter
		submitTestInput(t, run.inputs, externalEvent(t, 1, "queued", "continue"))
		submitTestInput(t, run.inputs, stopInput(t, "stop", inbox.StopWhenIdle))
		run.start(t)
		if err := <-run.done; err != nil {
			t.Fatal(err)
		}
		requests := adapter.requestSnapshot()
		want := withPreamble(t,
			llm.Item{Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleUser, Text: "hello"}},
			llm.Item{Type: llm.ItemMessage, Data: llm.Message{Role: llm.RoleUser, Text: "continue"}},
		)
		if len(requests) != 1 {
			t.Fatalf("requests = %d, want one resumed request", len(requests))
		}
		if !reflect.DeepEqual(requests[0].Input, want) {
			t.Fatal("resumed request did not include both inputs")
		}
	})
}

func TestCoordinatorResumeProcessesQueuedCompletionBeforeModelRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newStopTestRun(t, 1)
		run.store.items = append(run.store.items,
			storedItem(4, sessionstore.ItemInput, externalEvent(t, 0, "pending", "check progress")),
		)
		adapter := &fakeAdapter{respond: func(context.Context, llm.Request) (llm.Response, error) {
			return textResponse("Done."), nil
		}}
		run.current.dependencies.LLM = adapter
		completed := run.store.resume.Operations[0]
		completed.Status = operation.StatusCompleted
		run.operations.updates <- completed
		submitTestInput(t, run.inputs, stopInput(t, "stop", inbox.StopWhenIdle))
		run.start(t)
		if err := <-run.done; err != nil {
			t.Fatal(err)
		}
		requests := adapter.requestSnapshot()
		if len(requests) != 1 {
			t.Fatalf("requests = %d, want one resumed request", len(requests))
		}
		assertStopResult(t, requests[0], "call-0", string(operation.StatusCompleted))
	})
}
