//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() { fmt.Fprintln(os.Stderr, "qualification-owner requires Windows"); os.Exit(1) }
