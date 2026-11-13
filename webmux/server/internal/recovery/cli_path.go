package recovery

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func originalCLI(name string) (string, error) {
	search := os.Getenv("WEBMUX_BROWSER_ORIGINAL_PATH")
	if search == "" {
		search = os.Getenv("PATH")
	}
	missing := fmt.Errorf("%s is not installed on this execution host or is not available on its original PATH", name)
	if runtime.GOOS == "windows" {
		// Native adapters execute a program directly. PATHEXT may include cmd,
		// bat and scripts, which require a separate shell/argument contract.
		if filepath.Ext(name) == "" {
			name += ".exe"
		} else if !strings.EqualFold(filepath.Ext(name), ".exe") {
			return "", missing
		}
	}
	self, _ := os.Executable()
	selfInfo, _ := os.Stat(self)
	for _, dir := range filepath.SplitList(search) {
		if runtime.GOOS == "windows" && !filepath.IsAbs(dir) {
			// Never select a CLI from the current project via an empty/relative
			// PATH component (the same boundary as exec.LookPath's ErrDot).
			continue
		}
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode()&0111 == 0) {
			continue
		}
		if selfInfo != nil && os.SameFile(info, selfInfo) {
			continue
		}
		openerName := "webmux-open"
		if runtime.GOOS == "windows" {
			openerName += ".exe"
		}
		opener, openerErr := os.Stat(filepath.Join(dir, openerName))
		if openerErr == nil && (runtime.GOOS == "windows" || os.SameFile(info, opener)) {
			// Windows helper aliases are copies rather than Unix symlinks;
			// the dedicated opener directory must not resolve back into them.
			continue
		}
		if _, err := filepath.EvalSymlinks(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", missing
}
