package coordinator

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/viktordanov/uah-core/harness/contextbuilder"
	"github.com/viktordanov/uah-core/harness/llm"
	"github.com/viktordanov/uah-core/harness/operation"
	"github.com/viktordanov/uah-core/harness/tool"
)

const stillRunningFiveMinutes = "Still running after 5 minutes. The call continues in the background, and its result arrives in a later turn.\nOutput so far:\npartial"

func TestCoordinatorHoldWaitsForEveryCallOfTheTurn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newWakeTestRun(t, holdPolicy(5*time.Minute))
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
		synctest.Sleep(10 * time.Minute)
		if run.requestCount() != 2 {
			t.Fatal("the hold's valve opened for a call that had finished")
		}
	})
}

func TestCoordinatorHoldKeepsImmediateResults(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newWakeTestRun(t, holdPolicy(5*time.Minute))
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

func TestCoordinatorHoldEndsOnInboxInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newWakeTestRun(t, holdPolicy(5*time.Minute))
		run.respond(t, 0, toolGraceResponse("A", "B"))
		updateToolGraceCall(t, run, "A", operation.StatusCompleted)
		run.input(t, heartbeatInput(t, "heartbeat"))
		if run.requestCount() != 2 {
			t.Fatal("a heartbeat did not end the hold")
		}
		assertStopResult(t, run.calls[1].request, "B", contextbuilder.ToolCallRunningPayload)
	})
}

func TestCoordinatorHoldValveWakesWithOutputSoFar(t *testing.T) {
	for _, hold := range []time.Duration{5 * time.Minute, time.Minute} {
		t.Run(hold.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				run := newWakeTestRun(t, holdPolicy(hold))
				run.respond(t, 0, toolGraceResponse("A", "B"))
				updateToolGraceCall(t, run, "A", operation.StatusCompleted)
				synctest.Sleep(hold - 10*slurpIdleTimeout)
				if run.requestCount() != 1 {
					t.Fatal("the model woke before the valve opened")
				}
				synctest.Sleep(20 * slurpIdleTimeout)
				if run.requestCount() != 2 {
					t.Fatal("the valve did not wake the model")
				}
				assertStopResult(t, run.calls[1].request, "A", string(operation.StatusCompleted))
				assertStopResult(t, run.calls[1].request, "B", "Still running after "+durationText(hold)+". The call continues in the background, and its result arrives in a later turn.\nOutput so far:\npartial")
				run.respond(t, 1, textResponse("Waiting for B."))
				updateToolGraceCall(t, run, "B", operation.StatusCompleted)
				if run.requestCount() != 3 {
					t.Fatal("the call's result did not wake the model after the valve opened")
				}
				assertStopResult(t, run.calls[2].request, "B", string(operation.StatusCompleted))
			})
		})
	}
}

func TestCoordinatorHoldValveFreesLaterTurns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newWakeTestRun(t, holdPolicy(5*time.Minute))
		run.respond(t, 0, toolGraceResponse("A"))
		synctest.Sleep(5*time.Minute + 10*slurpIdleTimeout)
		if run.requestCount() != 2 {
			t.Fatal("the valve did not wake the model")
		}
		assertStopResult(t, run.calls[1].request, "A", stillRunningFiveMinutes)
		run.respond(t, 1, toolGraceResponse("C"))
		updateToolGraceCall(t, run, "C", operation.StatusCompleted)
		if run.requestCount() != 3 {
			t.Fatal("a call past its valve held a later turn")
		}
		run.respond(t, 2, textResponse("Waiting for A."))
		updateToolGraceCall(t, run, "A", operation.StatusCompleted)
		if run.requestCount() != 4 {
			t.Fatal("a call past its valve did not wake the model when it finished")
		}
	})
}

func TestDurationText(t *testing.T) {
	for d, want := range map[time.Duration]string{
		5 * time.Minute:  "5 minutes",
		time.Minute:      "1 minute",
		90 * time.Second: "90 seconds",
		time.Second:      "1 second",
	} {
		if got := durationText(d); got != want {
			t.Errorf("durationText(%s) = %q, want %q", d, got, want)
		}
	}
}

func newWakeTestRun(t *testing.T, policy WakePolicy) *stopTestRun {
	t.Helper()
	run := newToolGraceTestRun(t)
	run.current.dependencies.Wake = policy
	run.start(t)
	run.input(t, externalEvent(t, 0, "input", "run the tools"))
	return run
}

// holdPolicy holds for hold, with a progress of "partial" for each call.
func holdPolicy(hold time.Duration) WakePolicy {
	return WakePolicy{
		Hold: hold,
		Progress: func(operations []operation.Operation) string {
			if len(operations) != 1 {
				return ""
			}
			return "partial"
		},
	}
}
