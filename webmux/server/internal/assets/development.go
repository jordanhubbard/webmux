//go:build !embedded

package assets

import "io/fs"

// Plain go build/test supports backend development without a frontend build.
// Use go run ./cmd/build for a distributable, standalone executable.
func Web() fs.FS      { return nil }
func Defaults() fs.FS { return nil }
