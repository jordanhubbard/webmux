package openrigcoord

import "os/exec"

// Native Windows coordination hosting is refused by the HTTP boundary.
func configureChangeProcess(_ *exec.Cmd) {}
