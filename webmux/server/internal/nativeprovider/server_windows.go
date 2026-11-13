package nativeprovider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/screenstate"
)

type Terminal interface {
	io.ReadWriteCloser
	Resize(int, int) error
	Wait() (int, error)
	PID() int
}

type subscriber struct {
	connection net.Conn
	queue      chan frame
	done       chan struct{}
	once       sync.Once
}

func (s *subscriber) close() { s.once.Do(func() { close(s.done); _ = s.connection.Close() }) }

type owner struct {
	identity    Identity
	key         string
	terminal    Terminal
	mu          sync.Mutex
	resizeMu    sync.Mutex
	history     history
	screen      *screenstate.Screen
	screenError error
	changed     chan struct{}
	viewers     map[*subscriber]bool
	exit        *frame
}

func (o *owner) publish(f frame) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(f.Data) > 0 {
		o.history.add(f.Data)
		if o.screenError == nil {
			o.screenError = o.screen.Feed(f.Data)
		}
	}
	if f.Kind == "exit" {
		saved := f
		o.exit = &saved
	}
	close(o.changed)
	o.changed = make(chan struct{})
	for viewer := range o.viewers {
		select {
		case viewer.queue <- f:
		default:
			viewer.close()
			delete(o.viewers, viewer)
		}
	}
}

func (o *owner) resize(cols, rows int) error {
	o.resizeMu.Lock()
	defer o.resizeMu.Unlock()
	// Match the viewer before asking ConPTY to repaint. Never hold the owner
	// lock during native I/O, which may need its output drained to complete.
	o.mu.Lock()
	err := o.screen.Resize(cols, rows)
	if err != nil {
		o.screenError = err
	}
	o.mu.Unlock()
	if err != nil {
		return err
	}
	return o.terminal.Resize(cols, rows)
}

func (o *owner) capture(ctx context.Context, viewer *subscriber) (screenstate.Image, error) {
	o.resizeMu.Lock()
	defer o.resizeMu.Unlock()
	for {
		o.mu.Lock()
		if len(o.viewers) >= 8 || o.exit != nil {
			o.mu.Unlock()
			return screenstate.Image{}, errors.New("native provider is unavailable for attachment")
		}
		if o.screenError != nil {
			o.mu.Unlock()
			return screenstate.Image{}, errors.New("native terminal snapshot is unavailable; retained logs remain available")
		}
		image, err := o.screen.Capture()
		if err == nil {
			// Register at the same cut as the image. Subsequent output queues
			// while encoding/sending it; none is lost or sent ahead of it.
			o.viewers[viewer] = true
			o.mu.Unlock()
			return image, nil
		}
		changed := o.changed
		o.mu.Unlock()
		if !errors.Is(err, screenstate.ErrIncomplete) {
			return screenstate.Image{}, err
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return screenstate.Image{}, screenstate.ErrIncomplete
		}
	}
}

func (o *owner) handle(connection net.Conn) {
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(2 * time.Second))
	var request frame
	if readFrame(connection, &request) != nil {
		return
	}
	if request.Protocol != 1 || request.Session != o.identity.Session || request.Instance != o.identity.Instance || !equal(request.Key, o.key) {
		_ = writeFrame(connection, frame{Kind: "rejected", Error: "native provider authentication failed"})
		return
	}
	switch request.Kind {
	case "probe":
		o.mu.Lock()
		exited := o.exit != nil
		o.mu.Unlock()
		if exited {
			_ = writeFrame(connection, frame{Kind: "rejected", Error: "native provider has exited"})
		} else {
			_ = writeFrame(connection, frame{Kind: "ready", Identity: &o.identity})
		}
		return
	case "stop":
		_ = writeFrame(connection, frame{Kind: "stopping", Identity: &o.identity})
		_ = o.terminal.Close()
		return
	case "attach":
	default:
		_ = writeFrame(connection, frame{Kind: "rejected", Error: "unsupported native provider operation"})
		return
	}
	if !validSize(request.Cols, request.Rows) {
		return
	}
	_ = connection.SetDeadline(time.Time{})
	viewer := &subscriber{connection: connection, queue: make(chan frame, 64), done: make(chan struct{})}
	defer viewer.close()
	if err := o.resize(request.Cols, request.Rows); err != nil {
		_ = writeFrame(connection, frame{Kind: "rejected", Error: "native terminal resize failed"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	image, err := o.capture(ctx, viewer)
	cancel()
	if err != nil {
		_ = writeFrame(connection, frame{Kind: "rejected", Error: err.Error()})
		return
	}
	defer func() { o.mu.Lock(); delete(o.viewers, viewer); o.mu.Unlock() }()
	snapshot, err := image.Bytes()
	if err != nil {
		_ = writeFrame(connection, frame{Kind: "rejected", Error: err.Error()})
		return
	}
	// A snapshot may exceed the live queue's capacity. Send it first with a
	// total deadline, while the bounded queue captures only subsequent output.
	_ = connection.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if writeFrame(connection, frame{Kind: "attached", Identity: &o.identity}) != nil {
		return
	}
	for len(snapshot) > 0 {
		n := min(len(snapshot), maxData)
		if writeFrame(connection, frame{Kind: "data", Data: snapshot[:n]}) != nil {
			return
		}
		snapshot = snapshot[n:]
	}
	go func() {
		defer viewer.close()
		for {
			select {
			case <-viewer.done:
				return
			case reply := <-viewer.queue:
				_ = connection.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if writeFrame(connection, reply) != nil || reply.Kind == "exit" {
					return
				}
			}
		}
	}()
	for {
		// An idle attached viewer has no read deadline. A connection which
		// begins a frame must finish it promptly and within the length bound.
		var first [1]byte
		if _, err := io.ReadFull(connection, first[:]); err != nil {
			return
		}
		_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
		var message frame
		if readFrame(io.MultiReader(bytes.NewReader(first[:]), connection), &message) != nil {
			return
		}
		_ = connection.SetReadDeadline(time.Time{})
		if message.Identity != nil || message.Key != "" || message.Session != "" || message.Instance != "" {
			return
		}
		switch message.Kind {
		case "input":
			if len(message.Data) > maxData {
				return
			}
			if n, err := o.terminal.Write(message.Data); err != nil || n != len(message.Data) {
				return
			}
		case "resize":
			if !validSize(message.Cols, message.Rows) {
				return
			}
			if err := o.resize(message.Cols, message.Rows); err != nil {
				return
			}
		default:
			return
		}
	}
}

// Serve owns the lifetime mutex and the ConPTY/job, not the startup controller.
// Losing the startup reply or every viewer never terminates an established job.
func Serve(ctx context.Context, input io.Reader, output io.Writer, start func(Launch) (Terminal, error)) error {
	var c Config
	if err := readFrame(input, &c); err != nil {
		return err
	}
	if err := validate(c); err != nil {
		return err
	}
	if err := prepareRoot(c.Root); err != nil {
		return err
	}
	release, err := ownSession(c.Session)
	if err != nil {
		_ = writeFrame(output, frame{Kind: "ready", Error: err.Error()})
		return nil
	}
	defer release()
	dir, err := os.MkdirTemp("", "wmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := secureCreatedDirectory(dir); err != nil {
		return err
	}
	endpoint := filepath.Join(dir, "s")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: endpoint, Net: "unix"})
	if err != nil {
		return err
	}
	defer listener.Close()
	id, err := instance()
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	image, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(image)
	screen, err := screenstate.New(c.Launch.Cols, c.Launch.Rows)
	if err != nil {
		return err
	}
	defer screen.Close()
	o := &owner{identity: Identity{Session: c.Session, Instance: id, Endpoint: endpoint, OwnerPID: os.Getpid(), BinarySHA256: hex.EncodeToString(digest[:]), Replay: "screen-v1"}, key: c.Key, viewers: map[*subscriber]bool{}, screen: screen, changed: make(chan struct{})}
	if err := writeRecord(c.Root, c.Key, o.identity); err != nil {
		return err
	}
	defer os.Remove(filepath.Join(c.Root, "identity.json"))
	p, err := start(c.Launch)
	if err != nil {
		_ = writeFrame(output, frame{Kind: "ready", Error: "native terminal creation failed"})
		return err
	}
	o.terminal = p
	defer p.Close()
	o.identity.RootPID = p.PID()
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		b := make([]byte, maxData)
		for {
			n, err := p.Read(b)
			if n > 0 {
				o.publish(frame{Kind: "data", Data: append([]byte(nil), b[:n]...)})
			}
			if err != nil {
				return
			}
		}
	}()
	if err := writeRecord(c.Root, c.Key, o.identity); err != nil {
		return err
	}
	// A broken startup pipe means its caller disappeared, not that the job
	// disappeared. The authenticated record remains the recovery rendezvous.
	_ = writeFrame(output, frame{Kind: "ready", Identity: &o.identity})
	if closer, ok := output.(io.Closer); ok {
		_ = closer.Close()
	}
	finished := make(chan struct{})
	go func() {
		code, err := p.Wait()
		<-readDone
		message := ""
		if err != nil {
			message = "native terminal wait failed"
		}
		o.publish(frame{Kind: "exit", Code: code, Error: message})
		_ = listener.Close()
		close(finished)
	}()
	go func() {
		select {
		case <-ctx.Done():
			_ = p.Close()
		case <-finished:
		}
	}()
	var handlers sync.WaitGroup
	limit := make(chan struct{}, 32)
	for {
		connection, err := listener.Accept()
		if err != nil {
			break
		}
		select {
		case limit <- struct{}{}:
			handlers.Add(1)
			go func() { defer handlers.Done(); defer func() { <-limit }(); o.handle(connection) }()
		default:
			_ = connection.Close()
		}
	}
	select {
	case <-finished:
	case <-ctx.Done():
		_ = p.Close()
		<-finished
	}
	handlers.Wait()
	return nil
}
