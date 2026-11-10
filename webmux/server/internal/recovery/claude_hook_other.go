//go:build !darwin && !linux

package recovery

func newClaudeObserver(string, []string) *claudeObserver { return nil }
