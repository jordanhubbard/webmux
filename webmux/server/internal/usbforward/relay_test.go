package usbforward

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func backend(t *testing.T, handle func(net.Conn)) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() { defer wg.Done(); defer c.Close(); handle(c) }()
		}
	}()
	t.Cleanup(func() { l.Close(); <-done; wg.Wait() })
	return l
}
func lease(t *testing.T, localPort, seconds int) (int, context.CancelFunc, <-chan error, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a, b := net.Pipe()
	workerDone, exportDone := make(chan error, 1), make(chan error, 1)
	go func() { workerDone <- Worker(ctx, b) }()
	port := make(chan int, 1)
	go func() {
		exportDone <- Export(ctx, a, localPort, Config{Version: 1, Seconds: seconds}, func(p int) { port <- p })
	}()
	select {
	case p := <-port:
		return p, cancel, workerDone, exportDone
	case err := <-exportDone:
		t.Fatalf("export setup: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("setup timed out")
	}
	return 0, nil, nil, nil
}
func dial(t *testing.T, port int) *net.TCPConn {
	t.Helper()
	c, err := net.DialTCP("tcp4", nil, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	t.Cleanup(func() { c.Close() })
	return c
}
func finished(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("relay: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("relay failed to stop")
	}
}

func TestDuplexHalfCloseAndRelease(t *testing.T) {
	payload := bytes.Repeat([]byte{0, 255, 1, 2, 0, 7}, 100000)
	l := backend(t, func(c net.Conn) {
		b, err := io.ReadAll(c)
		if err != nil {
			return
		}
		_, _ = c.Write(append([]byte("reply:"), b...))
	})
	port, cancel, w, e := lease(t, l.Addr().(*net.TCPAddr).Port, 10)
	c := dial(t, port)
	if _, err := c.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := c.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(c)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, append([]byte("reply:"), payload...)) {
		t.Fatalf("binary traffic corrupted: %d bytes", len(got))
	}
	cancel()
	finished(t, w)
	finished(t, e)
	probe, err := net.DialTimeout("tcp4", c.RemoteAddr().String(), 100*time.Millisecond)
	if err == nil {
		probe.Close()
		t.Fatal("listener survived cancellation")
	}
}
func TestConcurrentStreamsStaySeparate(t *testing.T) {
	l := backend(t, func(c net.Conn) { _, _ = io.Copy(c, c) })
	port, cancel, w, e := lease(t, l.Addr().(*net.TCPAddr).Port, 10)
	var wg sync.WaitGroup
	for i := range maxStreams {
		c := dial(t, port)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer c.Close()
			body := bytes.Repeat([]byte{byte(i)}, maxFrame*2+3)
			if _, err := c.Write(body); err != nil {
				t.Error(err)
				return
			}
			got := make([]byte, len(body))
			if _, err := io.ReadFull(c, got); err != nil {
				t.Error(err)
				return
			}
			if !bytes.Equal(body, got) {
				t.Error("cross-stream contamination")
			}
		}()
	}
	wg.Wait()
	cancel()
	finished(t, w)
	finished(t, e)
}
func TestConnectionLimitAndExpiry(t *testing.T) {
	arrivals := make(chan struct{}, maxStreams+1)
	l := backend(t, func(c net.Conn) { arrivals <- struct{}{}; _, _ = io.Copy(io.Discard, c) })
	port, _, w, e := lease(t, l.Addr().(*net.TCPAddr).Port, 2)
	for range maxStreams {
		dial(t, port)
		select {
		case <-arrivals:
		case <-time.After(time.Second):
			t.Fatal("stream did not connect")
		}
	}
	excess := dial(t, port)
	b := make([]byte, 1)
	if _, err := excess.Read(b); err == nil {
		t.Fatal("ninth stream unexpectedly accepted")
	} else if n, ok := err.(net.Error); ok && n.Timeout() {
		t.Fatal("ninth stream was not closed")
	}
	finished(t, w)
	finished(t, e)
}
func TestExporterLossClosesDeviceChannel(t *testing.T) {
	l := backend(t, func(c net.Conn) { b := make([]byte, 1); _, _ = c.Read(b) })
	port, cancel, w, e := lease(t, l.Addr().(*net.TCPAddr).Port, 10)
	c := dial(t, port)
	_, _ = c.Write([]byte{1})
	b := make([]byte, 1)
	if _, err := c.Read(b); !errors.Is(err, io.EOF) {
		t.Fatalf("want device EOF, got %v", err)
	}
	c.Close()
	cancel()
	finished(t, w)
	finished(t, e)
}
func TestRejectInvalidOptions(t *testing.T) {
	good := options{host: "me@build-mac", sshPort: 22, exportPort: 7575, duration: time.Minute}
	if err := good.validate(); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"-oProxyCommand=evil", "host;id", "$(id)", "host\nother", "a b", "host/other"} {
		o := good
		o.host = host
		if o.validate() == nil {
			t.Errorf("accepted host %q", host)
		}
	}
	for _, port := range []int{-1, 1, 1023, 65536} {
		o := good
		o.listenPort = port
		if o.validate() == nil {
			t.Errorf("accepted port %d", port)
		}
	}
	for _, d := range []time.Duration{0, time.Millisecond, 2 * time.Hour} {
		o := good
		o.duration = d
		if o.validate() == nil {
			t.Error("accepted lifetime", d)
		}
	}
	args := strings.Join(good.sshArgs(), " ")
	for _, required := range []string{"StrictHostKeyChecking=yes", "BatchMode=yes", "ForwardAgent=no", "ClearAllForwardings=yes", "ControlPath=none", "-- me@build-mac webmux --usb-forward-worker"} {
		if !strings.Contains(args, required) {
			t.Error("missing SSH constraint", required)
		}
	}
}
func TestWorkerRejectsBadFrames(t *testing.T) {
	for _, p := range []packet{{kind: setup, body: []byte(`{"version":1,"port":22,"seconds":30}`)}, {kind: ready}, {kind: setup, body: []byte(`{"version":2,"seconds":30}`)}} {
		a, b := net.Pipe()
		done := make(chan error, 1)
		go func() { done <- Worker(context.Background(), b) }()
		_ = (&wire{ReadWriteCloser: a}).send(p.kind, p.id, p.body)
		select {
		case err := <-done:
			if err == nil {
				t.Error("accepted bad handshake")
			}
		case <-time.After(time.Second):
			t.Fatal("worker stuck")
		}
		a.Close()
	}
	a, b := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- Worker(context.Background(), b) }()
	var h [9]byte
	h[0] = setup
	binary.BigEndian.PutUint32(h[5:], maxFrame+1)
	_, _ = a.Write(h[:])
	if err := <-done; !errors.Is(err, errProtocol) {
		t.Fatal(err)
	}
	a.Close()
}
func TestWorkerRejectsProtocolViolationAfterHandshake(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	done := make(chan error, 1)
	go func() { done <- Worker(context.Background(), b) }()
	wire := &wire{ReadWriteCloser: a}
	body, _ := json.Marshal(Config{Version: 1, Seconds: 10})
	_ = wire.send(setup, 0, body)
	p, err := wire.read()
	if err != nil || p.kind != ready {
		t.Fatal("no ready", err)
	}
	_ = wire.send(openStream, 1, nil) // only the listener may open streams
	select {
	case err := <-done:
		if !errors.Is(err, errProtocol) {
			t.Fatal("protocol error hidden", err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
}
func TestOccupiedPortAndCanceledHandshake(t *testing.T) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	a, b := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- Worker(context.Background(), b) }()
	body, _ := json.Marshal(Config{Version: 1, Port: l.Addr().(*net.TCPAddr).Port, Seconds: 10})
	_ = (&wire{ReadWriteCloser: a}).send(setup, 0, body)
	if err := <-done; err == nil {
		t.Fatal("occupied port was accepted")
	}
	a.Close()
	a, b = net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { done <- Worker(ctx, b) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancel did not interrupt handshake")
	}
	a.Close()
}
