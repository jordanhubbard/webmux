package terminal

import (
	"golang.org/x/sys/windows"
	"os"
)

func childBlockInput() error { return nil }

func childSize() (int, int, error) {
	var info windows.ConsoleScreenBufferInfo
	err := windows.GetConsoleScreenBufferInfo(windows.Handle(os.Stdout.Fd()), &info)
	return int(info.Size.X), int(info.Size.Y), err
}
