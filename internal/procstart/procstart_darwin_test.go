package procstart_test

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func currentBoot(t *testing.T) string {
	t.Helper()
	boot, err := unix.Sysctl("kern.bootsessionuuid")
	if err != nil {
		t.Fatal(err)
	}

	return boot
}

// checkStartedNow checks the start time against the clock and against ps,
// which reads it through another interface.
func checkStartedNow(t *testing.T, pid int, identity string) {
	t.Helper()
	_, start, _ := strings.Cut(identity, "/")
	sec, _, ok := strings.Cut(start, ".")
	if !ok {
		t.Fatalf("start %q is not <seconds>.<microseconds>", start)
	}
	n, err := strconv.ParseInt(sec, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Unix(n, 0)
	if d := time.Since(started); d < -time.Second || d > time.Minute {
		t.Fatalf("the child started %v ago", d)
	}
	out, err := exec.Command("/bin/ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(strings.Fields(string(out)), " "), started.Format("Mon Jan 2 15:04:05 2006"); got != want {
		t.Fatalf("ps says the child started %q, the identity says %q", got, want)
	}
}
