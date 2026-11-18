//go:build !linux || (!amd64 && !arm64)

package nativeobserve

import "errors"

func Run(args []string) error {
	return errors.New("native policy observation helper is unqualified on this platform")
}
