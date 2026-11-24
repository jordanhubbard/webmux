//go:build darwin || linux

package recovery

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
)

func newCodexSelectionObserver(binary, cwd string, args []string) *codexSelectionObserver {
	if !codexSelectionDirectExecutable(binary) || !codexSelectionEligible(binary, cwd, args) {
		return nil
	}
	return newCodexSelectionPipe()
}

// Script launchers may advertise the native child's help while closing extra
// descriptors when they spawn it. Never inject a private FD through an
// unqualified launcher: the native child would fail before opening its TUI.
func codexSelectionDirectExecutable(file string) bool {
	reader, err := os.Open(file)
	if err != nil {
		return false
	}
	defer reader.Close()
	var magic [4]byte
	if _, err := io.ReadFull(reader, magic[:]); err != nil {
		return false
	}
	switch binary.BigEndian.Uint32(magic[:]) {
	case 0x7f454c46, 0xfeedface, 0xcefaedfe, 0xfeedfacf, 0xcffaedfe,
		0xcafebabe, 0xbebafeca, 0xcafebabf, 0xbfbafeca:
		return true // ELF, thin Mach-O, or universal Mach-O.
	}
	return false
}

func newCodexSelectionPipe() *codexSelectionObserver {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil
	}
	observations := make(chan codexSelectionObservation, 1)
	done := make(chan struct{})
	var started, stopped sync.Once
	o := &codexSelectionObserver{observations: observations}
	o.attach = func(cmd *exec.Cmd) {
		fd := 3 + len(cmd.ExtraFiles)
		cmd.ExtraFiles = append(cmd.ExtraFiles, writer)
		cmd.Args = append([]string{cmd.Args[0], "--selection-observer-fd", strconv.Itoa(fd)}, cmd.Args[1:]...)
	}
	o.start = func(pid int) {
		started.Do(func() {
			_ = writer.Close() // only the exact child retains the write end
			go func() {
				defer close(done)
				defer reader.Close()
				publish := func(observation codexSelectionObservation) {
					// Store the newest state, including loss of confidence. A slow
					// consumer must not retain an older queued selection indefinitely.
					select {
					case <-observations:
					default:
					}
					select {
					case observations <- observation:
					default:
					}
				}
				scanner := bufio.NewScanner(reader)
				scanner.Buffer(make([]byte, 512), 513)
				var sequence uint64
				for scanner.Scan() {
					observation, valid := decodeCodexSelection(scanner.Bytes(), pid, sequence)
					if !valid {
						publish(codexSelectionObservation{})
						return
					}
					sequence = observation.Sequence
					publish(observation)
				}
				// Closed or malformed channels no longer witness the foreground.
				// In particular, never fall back to a launch selector after a switch.
				publish(codexSelectionObservation{})
			}()
		})
	}
	o.close = func() {
		stopped.Do(func() {
			_ = reader.Close()
			_ = writer.Close()
			o.start(0)
			<-done
		})
	}
	return o
}
