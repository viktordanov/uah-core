//go:build darwin || linux

package procstart_test

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/viktordanov/uah-core/internal/procstart"
)

func TestOf_IsStableForALiveProcess(t *testing.T) {
	first, err := procstart.Of(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	second, err := procstart.Of(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !strings.Contains(first, "/") {
		t.Fatalf("identities %q and %q, want one <boot>/<start>", first, second)
	}
	boot, _, _ := strings.Cut(first, "/")
	if want := currentBoot(t); boot != want {
		t.Fatalf("boot = %q, want %q", boot, want)
	}
}

func TestOf_TellsProcessesApart(t *testing.T) {
	self, err := procstart.Of(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	// Linux counts start times in clock ticks (10 ms): a process ID cannot be
	// reused that fast, but a child started at once can share its parent's tick.
	time.Sleep(30 * time.Millisecond)
	child := startSleep(t)
	other, err := procstart.Of(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	if other == self {
		t.Fatalf("a child has its parent's identity %q", self)
	}
	selfBoot, _, _ := strings.Cut(self, "/")
	otherBoot, _, _ := strings.Cut(other, "/")
	if selfBoot != otherBoot {
		t.Fatalf("boots %q and %q differ", selfBoot, otherBoot)
	}
	checkStartedNow(t, child.Process.Pid, other)
}

func TestOf_GoneProcess(t *testing.T) {
	child := exec.Command("/bin/sh", "-c", "exit 0")
	if err := child.Run(); err != nil {
		t.Fatal(err)
	}
	if id, err := procstart.Of(child.Process.Pid); !errors.Is(err, procstart.ErrNoProcess) {
		t.Fatalf("Of(reaped child) = %q, %v; want ErrNoProcess", id, err)
	}
	if _, err := procstart.Of(0); !errors.Is(err, procstart.ErrNoProcess) {
		t.Fatalf("Of(0) error = %v, want ErrNoProcess", err)
	}
}

func startSleep(t *testing.T) *exec.Cmd {
	t.Helper()
	child := exec.Command("/bin/sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	})

	return child
}
