package recovery

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Read only PIDs and parent PIDs, never other users' command lines. A hook from
// another terminal cannot publish even if it learns this invocation's endpoint.
func nativeProcessDescendant(pid, root int) bool {
	if pid < 1 || root < 1 {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=").Output()
	if err != nil || len(output) > 1<<20 {
		return false
	}
	parents := map[int]int{}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		child, e1 := strconv.Atoi(fields[0])
		parent, e2 := strconv.Atoi(fields[1])
		if e1 == nil && e2 == nil {
			parents[child] = parent
		}
	}
	for n := 0; n < 64 && pid > 1; n++ {
		if pid == root {
			return true
		}
		pid = parents[pid]
	}
	return pid == root
}
