//go:build embedded

// Package assets provides the read-only files compiled into release executables.
package assets

import (
	"embed"
	"io/fs"
)

// The build command stages these files in a private copy of the module.
//
//go:embed all:bundle/web all:bundle/config.defaults
var bundle embed.FS

func Web() fs.FS {
	files, _ := fs.Sub(bundle, "bundle/web")
	return files
}

func Defaults() fs.FS {
	files, _ := fs.Sub(bundle, "bundle/config.defaults")
	return files
}
