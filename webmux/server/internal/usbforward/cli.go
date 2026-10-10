package usbforward

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Stdio is used only by the explicit worker invocation, never the HTTP server.
type Stdio struct {
	In  io.ReadCloser
	Out io.WriteCloser
}

func (s Stdio) Read(p []byte) (int, error)  { return s.In.Read(p) }
func (s Stdio) Write(p []byte) (int, error) { return s.Out.Write(p) }
func (s Stdio) Close() error                { return errors.Join(s.In.Close(), s.Out.Close()) }

type options struct {
	host, identity                  string
	sshPort, exportPort, listenPort int
	duration                        time.Duration
}

var destination = regexp.MustCompile(`^([a-zA-Z0-9_][a-zA-Z0-9_.-]*@)?[a-zA-Z0-9_][a-zA-Z0-9_.-]*$`)

func (o options) validate() error {
	if !destination.MatchString(o.host) || len(o.host) > 253 {
		return errors.New("--host must be a known SSH hostname or alias, optionally user@host")
	}
	if o.sshPort < 1 || o.sshPort > 65535 || o.exportPort < 1024 || o.exportPort > 65535 {
		return errors.New("invalid SSH port or loopback exporter port")
	}
	if o.duration < time.Second || o.duration > maxLifetime || o.duration%time.Second != 0 {
		return errors.New("--duration must be whole seconds, from 1s to 1h")
	}
	if strings.ContainsAny(o.identity, "\r\n\x00") {
		return errors.New("invalid identity path")
	}
	return (Config{Version: 1, Port: o.listenPort, Seconds: int(o.duration.Seconds())}).validate()
}
func (o options) sshArgs() []string {
	// No remote shell input is built from user data: only this fixed helper command.
	// SSH configuration may select an alias, jump host or local key/agent. It may
	// not add a PTY, forwards, local commands or a shared control connection.
	a := []string{"-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=10", "-o", "ServerAliveCountMax=2", "-o", "ForwardAgent=no",
		"-o", "ForwardX11=no", "-o", "ClearAllForwardings=yes", "-o", "PermitLocalCommand=no",
		"-o", "ControlMaster=no", "-o", "ControlPath=none", "-o", "RequestTTY=no",
		"-p", strconv.Itoa(o.sshPort)}
	if o.identity != "" {
		a = append(a, "-i", o.identity)
	}
	return append(a, "--", o.host, "webmux --usb-forward-worker")
}

// Run starts a foreground lease on the machine physically hosting the USB device.
// Driver installation, USB selection/authorization and signing remain native
// owner actions outside this transport helper.
func Run(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("webmux usb-forward", flag.ContinueOnError)
	fs.SetOutput(out)
	var o options
	fs.StringVar(&o.host, "host", "", "development Mac SSH host or user@host (known host key required)")
	fs.StringVar(&o.identity, "identity", "", "optional existing SSH private-key path")
	fs.IntVar(&o.sshPort, "ssh-port", 22, "SSH port")
	fs.IntVar(&o.exportPort, "export-port", 7575, "local USB exporter TCP port on 127.0.0.1")
	fs.IntVar(&o.listenPort, "listen-port", 0, "development host loopback port (0 chooses a free port)")
	fs.DurationVar(&o.duration, "duration", 30*time.Minute, "foreground forwarding lease, maximum 1h")
	fs.Usage = func() {
		fmt.Fprintln(out, "Experimental USB transport. Run beside the physical device with a separately configured native USB exporter; matching WebMux and a USB importer are required on the development Mac. This does not establish iPhone/Xcode compatibility.\nUsage: webmux usb-forward --host user@build-mac [options]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if err := o.validate(); err != nil {
		return err
	}
	// A reachable loopback service is a prerequisite, not proof that it is a USB
	// exporter or that a physical device is shared. No bytes are sent by this probe.
	probe, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(o.exportPort)))
	if err != nil {
		return errors.New("no loopback USB exporter at the selected port; configure and start the native exporter first")
	}
	probe.Close()
	ctx, cancel := context.WithTimeout(ctx, o.duration)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", o.sshArgs()...)
	cmd.Stderr = out
	cmd.WaitDelay = 3 * time.Second
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return err
	}
	if err := cmd.Start(); err != nil {
		input.Close()
		output.Close()
		return fmt.Errorf("start SSH: %w", err)
	}
	transport := Stdio{In: output, Out: input}
	err = Export(ctx, transport, o.exportPort, Config{Version: 1, Port: o.listenPort, Seconds: int(o.duration.Seconds())}, func(port int) {
		fmt.Fprintf(out, "USB transport ready on development host 127.0.0.1:%d for at most %s. Select this address in the native USB importer. Device/Xcode availability is not yet verified. Press Ctrl-C to release.\n", port, o.duration)
	})
	expired := ctx.Err() != nil
	cancel() // also ends the dedicated SSH process if the relay encountered an error
	waitErr := cmd.Wait()
	if expired {
		fmt.Fprintln(out, "USB forwarding lease ended; connections released.")
		return nil
	}
	if err != nil {
		return err
	}
	if waitErr != nil {
		return fmt.Errorf("SSH USB forwarding ended: %w", waitErr)
	}
	fmt.Fprintln(out, "USB forwarding ended; connections released.")
	return nil
}

// Check for a pipe instead of allowing the binary worker to emit frames into an
// interactive terminal accidentally. SSH invokes it with redirected stdin.
func WorkerStdio(ctx context.Context) error {
	info, err := os.Stdin.Stat()
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		return errors.New("USB worker requires a dedicated SSH stdio channel")
	}
	return Worker(ctx, Stdio{In: os.Stdin, Out: os.Stdout})
}
