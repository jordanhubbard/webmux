package usbforward

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRealSSHTransportAndHostKeyRejection(t *testing.T) {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("OpenSSH client is not installed")
	}
	// Exercise the shipped argument parser, exporter and worker processes too.
	binaryPath := filepath.Join(t.TempDir(), "webmux")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}
	build := exec.Command("go", "build", "-o", binaryPath, "../../cmd/webmux")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v: %s", err, output)
	}
	_, hostKey, _ := ed25519.GenerateKey(rand.Reader)
	hostSigner, _ := ssh.NewSignerFromKey(hostKey)
	pub, clientKey, _ := ed25519.GenerateKey(rand.Reader)
	clientPub, _ := ssh.NewPublicKey(pub)
	config := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if !bytes.Equal(clientPub.Marshal(), key.Marshal()) {
			return nil, errProtocol
		}
		return nil, nil
	}}
	config.AddHostKey(hostSigner)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var workers sync.WaitGroup
	accepted := make(chan struct{})
	go func() {
		defer close(accepted)
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer c.Close()
				conn, channels, requests, err := ssh.NewServerConn(c, config)
				if err != nil {
					return
				}
				defer conn.Close()
				go ssh.DiscardRequests(requests)
				for ch := range channels {
					if ch.ChannelType() != "session" {
						_ = ch.Reject(ssh.UnknownChannelType, "session only")
						continue
					}
					stream, reqs, err := ch.Accept()
					if err != nil {
						return
					}
					for req := range reqs {
						var command struct{ Command string }
						if req.Type != "exec" || ssh.Unmarshal(req.Payload, &command) != nil || command.Command != "webmux --usb-forward-worker" {
							_ = req.Reply(false, nil)
							continue
						}
						if conn.User() == "fail-worker" {
							_ = req.Reply(false, nil)
							return
						}
						_ = req.Reply(true, nil)
						worker := exec.CommandContext(ctx, binaryPath, "--usb-forward-worker")
						worker.Stdin, worker.Stdout, worker.Stderr = stream, stream, stream.Stderr()
						worker.WaitDelay = time.Second
						_ = worker.Run()
						_ = stream.Close()
						return
					}
				}
			}()
		}
	}()
	defer func() { cancel(); listener.Close(); <-accepted; workers.Wait() }()
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "fixture-key")
	known := filepath.Join(dir, "known-hosts")
	block, err := ssh.MarshalPrivateKey(clientKey, "temporary test key")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(keyFile, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	hostLine := "[127.0.0.1]:" + strconv.Itoa(listener.Addr().(*net.TCPAddr).Port) + " " + string(ssh.MarshalAuthorizedKey(hostSigner.PublicKey()))
	if err = os.WriteFile(known, []byte(hostLine), 0600); err != nil {
		t.Fatal(err)
	}
	exporter := backend(t, func(c net.Conn) { _, _ = io.Copy(c, c) })
	for _, failure := range []string{"exporter-unavailable", "unknown-host", "changed-host", "worker-startup"} {
		t.Run("cli-"+failure, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			exportPort := exporter.Addr().(*net.TCPAddr).Port
			trust, host := known, "fixture@127.0.0.1"
			if failure == "exporter-unavailable" {
				closed, err := net.Listen("tcp4", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				exportPort = closed.Addr().(*net.TCPAddr).Port
				closed.Close()
			}
			if failure == "unknown-host" || failure == "changed-host" {
				trust = filepath.Join(t.TempDir(), "known-hosts")
				var contents []byte
				if failure == "changed-host" {
					contents = []byte("[127.0.0.1]:" + strconv.Itoa(listener.Addr().(*net.TCPAddr).Port) + " " + string(ssh.MarshalAuthorizedKey(clientPub)))
				}
				if err := os.WriteFile(trust, contents, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if failure == "worker-startup" {
				host = "fail-worker@127.0.0.1"
			}
			cmd := exec.CommandContext(ctx, binaryPath, "usb-forward", "--host", host,
				"--identity", keyFile, "--known-hosts", trust, "--ssh-port", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port),
				"--export-port", strconv.Itoa(exportPort), "--duration", "20s")
			output, err := cmd.CombinedOutput()
			if err == nil || ctx.Err() != nil || strings.Contains(string(output), "USB transport ready") {
				t.Fatalf("CLI must fail before readiness: err=%v context=%v output=%s", err, ctx.Err(), output)
			}
			if failure == "exporter-unavailable" && !strings.Contains(string(output), "configure and start the native exporter") {
				t.Fatalf("missing exporter mitigation: %s", output)
			}
		})
	}
	for _, trusted := range []bool{true, false} {
		t.Run(strconv.FormatBool(trusted), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			hostsFile := known
			if !trusted {
				hostsFile = filepath.Join(dir, "empty-hosts")
				if err := os.WriteFile(hostsFile, nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			o := options{host: "fixture@127.0.0.1", identity: keyFile, sshPort: listener.Addr().(*net.TCPAddr).Port}
			args := append([]string{"-F", os.DevNull, "-o", "UserKnownHostsFile=" + hostsFile, "-o", "GlobalKnownHostsFile=" + os.DevNull, "-o", "IdentitiesOnly=yes", "-o", "IdentityAgent=none"}, o.sshArgs()...)
			cmd := exec.CommandContext(ctx, sshPath, args...)
			cmd.WaitDelay = time.Second
			var diagnostic bytes.Buffer
			cmd.Stderr = &diagnostic
			in, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			out, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			// Reap the child on every path, including t.Fatal. Only read stderr
			// after Wait has joined the exec copy goroutine.
			var waitOnce sync.Once
			var waitErr error
			wait := func() error {
				waitOnce.Do(func() { waitErr = cmd.Wait() })
				return waitErr
			}
			defer func() { cancel(); _ = in.Close(); _ = out.Close(); _ = wait() }()
			ports := make(chan int, 1)
			done := make(chan error, 1)
			go func() {
				done <- Export(ctx, Stdio{In: out, Out: in}, exporter.Addr().(*net.TCPAddr).Port, Config{Version: 1, Seconds: 30}, func(port int) { ports <- port })
			}()
			if !trusted {
				select {
				case err := <-done:
					if err == nil {
						t.Error("untrusted host was accepted")
					}
				case <-ports:
					t.Error("untrusted host opened a listener")
				case <-ctx.Done():
					t.Error("host rejection timed out")
				}
				if wait() == nil {
					t.Error("OpenSSH accepted an unknown host key")
				}
				return
			}
			select {
			case port := <-ports:
				c := dial(t, port)
				payload := []byte{0, 255, 42, 13, 10}
				_, _ = c.Write(payload)
				got := make([]byte, len(payload))
				_, err = io.ReadFull(c, got)
				c.Close()
				if err != nil || !bytes.Equal(payload, got) {
					t.Error("SSH byte relay failed", err)
				}
			case err := <-done:
				_ = wait()
				t.Fatal("SSH relay ended early", err, diagnostic.String())
			case <-ctx.Done():
				cancel()
				_ = wait()
				t.Fatalf("SSH relay did not become ready: %v; stderr: %s", ctx.Err(), diagnostic.String())
			}
			cancel()
			finished(t, done)
			_ = wait()
		})
	}
	t.Run("cli-lease-and-cleanup", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		cliKnownHosts := filepath.Join(t.TempDir(), "known hosts")
		trustedKeys, err := os.ReadFile(known)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cliKnownHosts, trustedKeys, 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, binaryPath, "usb-forward", "--host", "fixture@127.0.0.1",
			"--identity", keyFile, "--known-hosts", cliKnownHosts, "--ssh-port", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port),
			"--export-port", strconv.Itoa(exporter.Addr().(*net.TCPAddr).Port), "--duration", "10s")
		var diagnostic bytes.Buffer
		out, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stderr = cmd.Stdout
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		var once sync.Once
		var waitErr error
		wait := func() error { once.Do(func() { waitErr = cmd.Wait() }); return waitErr }
		defer func() { cancel(); _ = out.Close(); _ = wait() }()
		ports := make(chan int, 1)
		readDone := make(chan struct{})
		go func() {
			defer close(readDone)
			scanner := bufio.NewScanner(out)
			for scanner.Scan() {
				line := scanner.Text()
				diagnostic.WriteString(line + "\n")
				if strings.HasPrefix(line, "USB transport ready") {
					var port int
					if _, err := fmt.Sscanf(line, "USB transport ready on development host 127.0.0.1:%d", &port); err == nil {
						ports <- port
					}
				}
			}
		}()
		var port int
		select {
		case port = <-ports:
		case <-readDone:
			_ = wait()
			t.Fatalf("CLI failed: %v: %s", waitErr, diagnostic.String())
		case <-ctx.Done():
			cancel()
			_ = wait()
			<-readDone
			t.Fatalf("CLI startup timed out: %s", diagnostic.String())
		}
		c := dial(t, port)
		payload := bytes.Repeat([]byte{0, 255, 10, 42}, 20000)
		go func() { _, _ = c.Write(payload) }()
		got := make([]byte, len(payload))
		if _, err := io.ReadFull(c, got); err != nil || !bytes.Equal(payload, got) {
			t.Fatalf("CLI payload mismatch: %v", err)
		}
		c.Close()
		<-readDone // The real lease must end the CLI without an external kill.
		if err := wait(); err != nil {
			t.Fatalf("CLI lease cleanup: %v: %s", err, diagnostic.String())
		}
		probe, err := net.DialTimeout("tcp4", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
		if err == nil {
			probe.Close()
			t.Fatal("worker listener survived CLI lease expiry")
		}
	})

}
