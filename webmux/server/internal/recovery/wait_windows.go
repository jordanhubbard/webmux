//go:build windows

package recovery

import (
	"context"
	"errors"
)

func WaitForRecovery(context.Context) error {
	return errors.New("native Windows recovery waiting has not been qualified; reconnect manually")
}
