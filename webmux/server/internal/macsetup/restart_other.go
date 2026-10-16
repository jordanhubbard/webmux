//go:build !darwin

package macsetup

import "errors"

func Replace() error { return errors.New("guided restart is only available on macOS") }
