package coordinator

import (
	"encoding/json/v2"
	"testing"
	"testing/synctest"
	"time"

	"github.com/viktordanov/unreal-agent/harness/contextbuilder"
	"github.com/viktordanov/unreal-agent/harness/llm"
	"github.com/viktordanov/unreal-agent/harness/operation"
	"github.com/viktordanov/unreal-agent/harness/tool"
)

func TestCoordinatorBatchWaitsForEveryCallOfTheTurn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newWakeTestRun(t, WakePolicy{Batch: true})
		run.respond(t, 0, toolGraceResponse("A", "B"))
		updateToolGraceCall(t, run, "A", operation.StatusCompleted)
		synctest.Sleep(time.Minute)
		if run.requestCount() != 1 {
			t.Fatal("a result woke the model while the turn's other call ran")
		}
		updateToolGraceCall(t, run, "B", operation.StatusCompleted)
		if run.requestCount() != 2 {
			t.Fatal("the turn's last result did not wake the model")
		}
		assertStopResult(t, run.calls[1].request, "A", string(operation.StatusCompleted))
		assertStopResult(t, run.calls[1].request, "B", string(operation.StatusCompleted))
	})
}

func TestCoordinatorBatchHoldsImmediateResults(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newWakeTestRun(t, WakePolicy{Batch: true})
		response := toolGraceResponse("A")
		response.Output = append(response.Output, llm.Item{Type: llm.ItemToolCall, Data: llm.ToolCall{
			CallID: "immediate", Name: tool.ViewImageName, Arguments: `{}`,
		}})
		run.respond(t, 0, response)
		if run.requestCount() != 1 {
			t.Fatal("an immediate result woke the model while the turn's other call ran")
		}
		updateToolGraceCall(t, run, "A", operation.StatusCompleted)
		if run.requestCount() != 2 {
			t.Fatal("the turn's last result did not wake the model")
		}
		assertStopResult(t, run.calls[1].request, "immediate", "")
		assertStopResult(t, run.calls[1].request, "A", string(operation.StatusCompleted))
	})
}

func TestCoordinatorBatchEndsOnInboxInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newWakeTestRun(t, WakePolicy{Batch: true})
		run.respond(t, 0, toolGraceResponse("A", "B"))
		updateToolGraceCall(t, run, "A", operation.StatusCompleted)
		run.input(t, heartbeatInput(t, "heartbeat"))
		if run.requestCount() != 2 {
			t.Fatal("a heartbeat did not end the batch")
		}
		assertStopResult(t, run.calls[1].request, "B", contextbuilder.ToolCallRunningPayload)
	})
}

func TestCoordinatorDebounceGathersResultsThatLandTogether(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newWakeTestRun(t, WakePolicy{Debounce: 2 * time.Second})
		run.respond(t, 0, toolGraceResponse("A", "B", "C"))
		synctest.Sleep(toolCallRunGracePeriod)
		updateToolGraceCall(t, run, "A", operation.StatusCompleted)
		deadline := time.Now().Add(2 * time.Second)
		synctest.Sleep(time.Second)
		updateToolGraceCall(t, run, "B", operation.StatusCompleted)
		synctest.Sleep(time.Until(deadline) - 10*slurpIdleTimeout)
		if run.requestCount() != 1 {
			t.Fatal("a result woke the model before the debounce window ended")
		}
		synctest.Sleep(20 * slurpIdleTimeout)
		if run.requestCount() != 2 {
			t.Fatal("the debounce window did not wake the model")
		}
		assertStopResult(t, run.calls[1].request, "A", string(operation.StatusCompleted))
		assertStopResult(t, run.calls[1].request, "B", string(operation.StatusCompleted))
		assertStopResult(t, run.calls[1].request, "C", contextbuilder.ToolCallRunningPayload)
		run.respond(t, 1, textResponse("Waiting for C."))
		updateToolGraceCall(t, run, "C", operation.StatusCompleted)
		if run.requestCount() != 3 {
			t.Fatal("the last running call's result was debounced")
		}
	})
}

func TestCoordinatorDebounceEndsWhenNothingRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newWakeTestRun(t, WakePolicy{Debounce: 2 * time.Second})
		run.respond(t, 0, toolGraceResponse("A", "B"))
		synctest.Sleep(toolCallRunGracePeriod)
		updateToolGraceCall(t, run, "A", operation.StatusCompleted)
		synctest.Sleep(500 * time.Millisecond)
		updateToolGraceCall(t, run, "B", operation.StatusCompleted)
		if run.requestCount() != 2 {
			t.Fatal("the debounce held results after every call finished")
		}
		assertStopResult(t, run.calls[1].request, "A", string(operation.StatusCompleted))
		assertStopResult(t, run.calls[1].request, "B", string(operation.StatusCompleted))
		synctest.Sleep(time.Minute)
		if run.requestCount() != 2 {
			t.Fatal("a stale debounce window woke the model")
		}
	})
}

func TestCoordinatorAllDoneSleepsUntilEveryCallFinishes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newWakeTestRun(t, WakePolicy{AllDone: true})
		run.respond(t, 0, toolGraceResponse("A", "B", "C"))
		synctest.Sleep(toolCallRunGracePeriod)
		updateToolGraceCall(t, run, "A", operation.StatusCompleted)
		if run.requestCount() != 2 {
			t.Fatal("a result did not wake the model while it had calls to make")
		}
		run.respond(t, 1, textResponse("Waiting for B and C."))
		updateToolGraceCall(t, run, "B", operation.StatusCompleted)
		synctest.Sleep(time.Minute)
		if run.requestCount() != 2 {
			t.Fatal("a result woke the model before every call finished")
		}
		updateToolGraceCall(t, run, "C", operation.StatusCompleted)
		if run.requestCount() != 3 {
			t.Fatal("the last result did not wake the model")
		}
		assertStopResult(t, run.calls[2].request, "B", string(operation.StatusCompleted))
		assertStopResult(t, run.calls[2].request, "C", string(operation.StatusCompleted))
	})
}

func TestCoordinatorAllDoneWakesOnFailureOrInput(t *testing.T) {
	for _, wake := range []string{"failed", "canceled", "steering", "heartbeat"} {
		t.Run(wake, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				run := newWakeTestRun(t, WakePolicy{AllDone: true})
				run.respond(t, 0, toolGraceResponse("A", "B"))
				synctest.Sleep(toolCallRunGracePeriod)
				run.input(t, externalEvent(t, 1, "check", "check in"))
				run.respond(t, 1, textResponse("Waiting."))
				switch wake {
				case "failed":
					updateToolGraceCall(t, run, "A", operation.StatusFailed)
				case "canceled":
					updateToolGraceCall(t, run, "A", operation.StatusCanceled)
				case "steering":
					run.input(t, externalEvent(t, 2, "steering", "status?"))
				case "heartbeat":
					run.input(t, heartbeatInput(t, "heartbeat"))
				}
				if run.requestCount() != 3 {
					t.Fatalf("%s did not wake a model sleeping until all calls finish", wake)
				}
				assertStopResult(t, run.calls[2].request, "B", contextbuilder.ToolCallRunningPayload)
			})
		})
	}
}

func TestCoordinatorAllDoneAppliesOnlyToTurnsWithoutCalls(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newWakeTestRun(t, WakePolicy{AllDone: true})
		run.respond(t, 0, toolGraceResponse("A", "B"))
		synctest.Sleep(toolCallRunGracePeriod)
		updateToolGraceCall(t, run, "A", operation.StatusCompleted)
		run.respond(t, 1, toolGraceResponse("C"))
		synctest.Sleep(toolCallRunGracePeriod)
		updateToolGraceCall(t, run, "C", operation.StatusCompleted)
		if run.requestCount() != 3 {
			t.Fatal("a turn that issued calls slept until every call finished")
		}
	})
}

func TestToolCallFailedSeesNonzeroExitCodes(t *testing.T) {
	exited := func(code int) operation.Operation {
		state, err := json.Marshal(operation.ShellState{Result: &operation.ShellResult{ExitCode: code}})
		if err != nil {
			t.Fatal(err)
		}
		return operation.Operation{
			Type: operation.TypeShell, Version: operation.VersionShell, MaxOutputLength: operation.DefaultMaxOutputLength,
			Status: operation.StatusCompleted, State: state,
		}
	}
	tests := []struct {
		name       string
		status     tool.CallStatus
		operations []operation.Operation
		want       bool
	}{
		{name: "success", operations: []operation.Operation{exited(0)}},
		{name: "nonzero exit", operations: []operation.Operation{exited(1)}, want: true},
		{name: "failed operation", operations: []operation.Operation{{Status: operation.StatusFailed}}, want: true},
		{name: "error", status: tool.ErrorStatus("bad arguments", 0), want: true},
	}
	for _, test := range tests {
		if got := toolCallFailed(test.status, test.operations); got != test.want {
			t.Errorf("%s: failed = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestCoordinatorYieldWaitsForForegroundCalls(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newWakeTestRun(t, yieldPolicy())
		run.respond(t, 0, toolGraceResponse("A", "background"))
		updateToolGraceCall(t, run, "background", operation.StatusCompleted)
		synctest.Sleep(20 * time.Second)
		if run.requestCount() != 1 {
			t.Fatal("a result woke the model while a foreground call ran")
		}
		updateToolGraceCall(t, run, "A", operation.StatusCompleted)
		if run.requestCount() != 2 {
			t.Fatal("the foreground call's result did not wake the model")
		}
		assertStopResult(t, run.calls[1].request, "A", string(operation.StatusCompleted))
		assertStopResult(t, run.calls[1].request, "background", string(operation.StatusCompleted))
	})
}

func TestCoordinatorYieldWakesWithOutputSoFar(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newWakeTestRun(t, yieldPolicy())
		run.respond(t, 0, toolGraceResponse("A", "background"))
		synctest.Sleep(30*time.Second - time.Millisecond)
		if run.requestCount() != 1 {
			t.Fatal("the model woke before the yield ended")
		}
		synctest.Sleep(time.Millisecond)
		if run.requestCount() != 2 {
			t.Fatal("the end of the yield did not wake the model")
		}
		assertStopResult(t, run.calls[1].request, "A", "Still running after 30s. The call continues in the background, and its result arrives in a later turn.\nOutput so far:\npartial")
		assertStopResult(t, run.calls[1].request, "background", contextbuilder.ToolCallRunningPayload)
		run.respond(t, 1, textResponse("Waiting."))
		updateToolGraceCall(t, run, "A", operation.StatusCompleted)
		if run.requestCount() != 3 {
			t.Fatal("a yielded call's result did not wake the model")
		}
		assertStopResult(t, run.calls[2].request, "A", string(operation.StatusCompleted))
	})
}

func TestCoordinatorYieldKeepsGraceForBackgroundCalls(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newWakeTestRun(t, yieldPolicy())
		run.respond(t, 0, toolGraceResponse("background", "background-2"))
		updateToolGraceCall(t, run, "background", operation.StatusCompleted)
		synctest.Sleep(toolCallRunGracePeriod)
		if run.requestCount() != 2 {
			t.Fatal("background calls did not keep the default grace period")
		}
		assertStopResult(t, run.calls[1].request, "background-2", contextbuilder.ToolCallRunningPayload)
	})
}

func newWakeTestRun(t *testing.T, policy WakePolicy) *stopTestRun {
	t.Helper()
	run := newToolGraceTestRun(t)
	run.current.dependencies.Wake = policy
	run.start(t)
	run.input(t, externalEvent(t, 0, "input", "run the tools"))
	return run
}

// yieldPolicy waits 30 seconds for each call but those named background.
func yieldPolicy() WakePolicy {
	return WakePolicy{
		Yield: func(call llm.ToolCall) time.Duration {
			if call.CallID == "background" || call.CallID == "background-2" {
				return 0
			}
			return 30 * time.Second
		},
		Progress: func(operations []operation.Operation) string {
			if len(operations) != 1 {
				return ""
			}
			return "partial"
		},
	}
}
