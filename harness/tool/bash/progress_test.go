package bash_test

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/viktordanov/unreal-agent/harness/operation"
	"github.com/viktordanov/unreal-agent/harness/tool/bash"
)

func TestProgressShowsCapturedOutputSoFar(t *testing.T) {
	base := t.TempDir()
	directory := filepath.Join(base, "operation-1")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("x", 5000) + "end"
	if err := os.WriteFile(filepath.Join(directory, operation.ShellOutFilename), []byte(long), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, operation.ShellErrFilename), []byte("warning\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := json.Marshal(operation.ShellState{BaseDirectory: base})
	if err != nil {
		t.Fatal(err)
	}
	running := operation.Operation{
		ID: "operation-1", Type: operation.TypeShell, Version: operation.VersionShell,
		MaxOutputLength: operation.DefaultMaxOutputLength, Status: operation.StatusAwaiting, State: state,
	}

	got := bash.Progress([]operation.Operation{running, {Type: operation.TypeValue}})
	want := "...1003 bytes before..." + long[1003:] + "\nStderr:\nwarning\n"
	if got != want {
		t.Fatalf("progress = %q, want %q", got, want)
	}
	if got := bash.Progress([]operation.Operation{{Type: operation.TypeValue}}); got != "" {
		t.Fatalf("progress of a non-shell operation = %q", got)
	}
}
