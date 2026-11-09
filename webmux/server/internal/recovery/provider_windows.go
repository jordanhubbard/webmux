//go:build windows

package recovery

import (
	"context"
	"errors"
)

func EnsureProvider(context.Context, string, string) (Event, error) {
	return Event{}, errors.New("native Windows provider recovery requires an available lifecycle provider")
}
