package usbforward

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestRealSSHTransportAndHostKeyRejection(t *testing.T) {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("OpenSSH client is not installed")
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
						_ = req.Reply(true, nil)
						_ = Worker(ctx, stream)
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
	for _, trusted := range []bool{true, false} {
		t.Run(strconv.FormatBool(trusted), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
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
			ports := make(chan int, 1)
			done := make(chan error, 1)
			go func() {
				done <- Export(ctx, Stdio{In: out, Out: in}, exporter.Addr().(*net.TCPAddr).Port, Config{Version: 1, Seconds: 5}, func(port int) { ports <- port })
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
				if cmd.Wait() == nil {
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
				_ = cmd.Wait()
				t.Fatal("SSH relay ended early", err, diagnostic.String())
			case <-ctx.Done():
				t.Fatal("SSH relay did not become ready")
			}
			cancel()
			finished(t, done)
			_ = cmd.Wait()
		})
	}
}
