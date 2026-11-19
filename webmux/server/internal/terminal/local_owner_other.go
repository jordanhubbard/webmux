//go:build !windows

package terminal

import (
	"context"
	"errors"
)

func LocalExecutionScope() (string, error) {
	return "", errors.New("native local owner scope requires Windows")
}

func StopLocalOwner(context.Context, LocalOwnerTarget, string) error {
	return errors.New("native local owner stop requires its Windows execution host")
}

func InspectLocalOwner(context.Context, LocalOwnerTarget, string) error {
	return errors.New("native local owner inspection requires its Windows execution host")
}
