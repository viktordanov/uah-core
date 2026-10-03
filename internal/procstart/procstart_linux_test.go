package procstart_test

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func currentBoot(t *testing.T) string {
	t.Helper()
	id, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		t.Fatal(err)
	}

	return strings.TrimSpace(string(id))
}

// checkStartedNow converts the start time, in clock ticks after boot, to a
// wall time with the boot time from /proc/stat. USER_HZ is 100 on every
// Linux architecture Go supports.
func checkStartedNow(t *testing.T, _ int, identity string) {
	t.Helper()
	_, start, _ := strings.Cut(identity, "/")
	ticks, err := strconv.ParseInt(start, 10, 64)
	if err != nil {
		t.Fatalf("start %q is not clock ticks: %v", start, err)
	}
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		t.Fatal(err)
	}
	var btime int64
	for line := range strings.Lines(string(stat)) {
		if v, ok := strings.CutPrefix(line, "btime "); ok {
			btime, err = strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	started := time.Unix(btime, 0).Add(time.Duration(ticks) * time.Second / 100)
	if d := time.Since(started); d < -2*time.Second || d > time.Minute {
		t.Fatalf("the child started %v ago", d)
	}
}
