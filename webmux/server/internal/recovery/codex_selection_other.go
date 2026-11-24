//go:build !darwin && !linux

package recovery

func newCodexSelectionObserver(string, string, []string) *codexSelectionObserver { return nil }
