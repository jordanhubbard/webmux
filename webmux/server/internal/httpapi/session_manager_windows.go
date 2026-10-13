package httpapi

import "errors"

func verifyManagerSocket(_ string) error {
	return errors.New("session-manager local socket transport is not supported on Windows")
}
