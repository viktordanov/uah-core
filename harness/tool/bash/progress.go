package bash

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/viktordanov/unreal-agent/harness/operation"
)

// progressTail is how much of each stream Progress shows.
const progressTail = 4000

// Progress returns the tail of the output that running Bash operations have
// captured so far, or "" when there is none.
func Progress(operations []operation.Operation) string {
	var parts []string
	for _, value := range operations {
		if value.Type != operation.TypeShell {
			continue
		}
		out, errPath, err := operation.ShellCapturePaths(value)
		if err != nil {
			continue
		}
		if tail := readTail(out); tail != "" {
			parts = append(parts, tail)
		}
		if tail := readTail(errPath); tail != "" {
			parts = append(parts, "Stderr:\n"+tail)
		}
	}
	return strings.Join(parts, "\n")
}

// readTail returns the last progressTail bytes of the file at path, marking
// what it leaves out; a file it cannot read shows nothing.
func readTail(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return ""
	}
	skipped := max(info.Size()-progressTail, 0)
	data, err := io.ReadAll(io.NewSectionReader(file, skipped, info.Size()-skipped))
	if err != nil {
		return ""
	}
	if skipped > 0 {
		return fmt.Sprintf("...%d bytes before...%s", skipped, data)
	}
	return string(data)
}
