//go:build !windows

package recovery

import "io"

func newCodexProcessObserver(pid int) (func(string) (string, string), io.Closer) {
	return func(cwd string) (string, string) { return codexSessionForProcess(pid, cwd) }, nil
}

// RunCodexFilesHelper is unavailable on platforms with existing file observers.
func RunCodexFilesHelper() error { return nil }
