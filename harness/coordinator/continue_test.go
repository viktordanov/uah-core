package coordinator

import (
	"context"
	"fmt"
	"testing"
	"testing/synctest"

	"github.com/viktordanov/uah-core/harness/inbox"
	"github.com/viktordanov/uah-core/harness/llm"
)

func TestCoordinatorContinuesWhenIdle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newStopTestRun(t, 0)
		asked := 0
		run.current.dependencies.Continue = func(context.Context) []inbox.Input {
			asked++
			if asked > 2 {
				return nil
			}
			return []inbox.Input{externalEvent(t, asked, inbox.ID(fmt.Sprintf("more-%d", asked)), fmt.Sprintf("keep going %d", asked))}
		}
		run.start(t)
		run.input(t, externalEvent(t, 0, "input-0", "hello"), stopInput(t, "stop", inbox.StopWhenIdle))
		for index := range 3 {
			if len(run.calls) != index+1 {
				t.Fatalf("requests = %d, want %d ", len(run.calls), index+1)
			}
			run.respond(t, index, textResponse("done"))
			// A continuation takes one more read of the inbox.
			synctest.Sleep(2 * slurpIdleTimeout)
			synctest.Wait()
		}
		run.assertStopped(t)
		if asked != 3 {
			t.Fatalf("Continue asked %d times, want 3", asked)
		}
		last := run.calls[2].request.Input
		message, ok := last[len(last)-1].Data.(llm.Message)
		if !ok || message.Role != llm.RoleUser || message.Text != "keep going 2" {
			t.Fatalf("last request ends with %#v, want the second continuation", last[len(last)-1])
		}
	})
}

func TestCoordinatorHardStopDoesNotContinue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newStopTestRun(t, 0)
		run.current.dependencies.Continue = func(context.Context) []inbox.Input {
			t.Error("a hard stop asked for more input")
			return nil
		}
		run.start(t)
		run.input(t, externalEvent(t, 0, "input-0", "hello"))
		run.input(t, stopInput(t, "stop", inbox.StopHard))
		run.assertStopped(t)
	})
}

func TestCoordinatorRejectsInvalidContinueInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run := newStopTestRun(t, 0)
		run.current.dependencies.Continue = func(context.Context) []inbox.Input {
			return []inbox.Input{{Kind: inbox.InputExternal}}
		}
		run.start(t)
		run.input(t, stopInput(t, "stop", inbox.StopWhenIdle))
		select {
		case err := <-run.done:
			if err == nil {
				t.Fatal("Run accepted an input without an ID")
			}
		default:
			t.Fatal("Run did not stop")
		}
	})
}
