// Package usbforward carries a preconfigured loopback USB exporter's TCP
// protocol over a dedicated SSH stdio channel. It is not a USB driver.
package usbforward

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

const (
	maxFrame         = 32 * 1024
	maxStreams       = 8
	maxLifetime      = time.Hour
	setup       byte = iota + 1
	ready
	openStream
	data
	endWrite
	closeStream
)

var errProtocol = errors.New("invalid USB forwarding protocol")

type Config struct {
	Version int `json:"version"`
	Port    int `json:"port"`
	Seconds int `json:"seconds"`
}

func (c Config) validate() error {
	if c.Version != 1 || (c.Port != 0 && (c.Port < 1024 || c.Port > 65535)) || c.Seconds < 1 || c.Seconds > int(maxLifetime.Seconds()) {
		return errors.New("USB forwarding requires protocol 1, port 0 or 1024–65535, and a lifetime of 1–3600 seconds")
	}
	return nil
}

type packet struct {
	kind byte
	id   uint32
	body []byte
}
type wire struct {
	io.ReadWriteCloser
	mu sync.Mutex
}

func (w *wire) send(kind byte, id uint32, body []byte) error {
	if len(body) > maxFrame {
		return errProtocol
	}
	var header [9]byte
	header[0] = kind
	binary.BigEndian.PutUint32(header[1:5], id)
	binary.BigEndian.PutUint32(header[5:9], uint32(len(body)))
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := writeAll(w, header[:]); err != nil {
		return err
	}
	return writeAll(w, body)
}
func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}
func (w *wire) read() (packet, error) {
	var h [9]byte
	if _, err := io.ReadFull(w, h[:]); err != nil {
		return packet{}, err
	}
	size := binary.BigEndian.Uint32(h[5:])
	if size > maxFrame {
		return packet{}, errProtocol
	}
	p := packet{kind: h[0], id: binary.BigEndian.Uint32(h[1:5]), body: make([]byte, size)}
	_, err := io.ReadFull(w, p.body)
	return p, err
}

type stream struct {
	conn                net.Conn
	readDone, writeDone bool
}
type relay struct {
	ctx     context.Context
	cancel  context.CancelFunc
	w       *wire
	target  string // only the exporting side may open connections, to this fixed loopback target
	mu      sync.Mutex
	streams map[uint32]*stream
	readers sync.WaitGroup
}

func newRelay(ctx context.Context, w *wire, target string) *relay {
	ctx, cancel := context.WithCancel(ctx)
	return &relay{ctx: ctx, cancel: cancel, w: w, target: target, streams: make(map[uint32]*stream)}
}
func (r *relay) add(id uint32, c net.Conn) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ctx.Err() != nil || len(r.streams) >= maxStreams || r.streams[id] != nil {
		return false
	}
	r.streams[id] = &stream{conn: c}
	return true
}
func (r *relay) remove(id uint32) {
	r.mu.Lock()
	s := r.streams[id]
	delete(r.streams, id)
	r.mu.Unlock()
	if s != nil {
		s.conn.Close()
	}
}
func (r *relay) halfDone(id uint32, reading bool) {
	r.mu.Lock()
	s := r.streams[id]
	closeNow := false
	if s != nil {
		if reading {
			s.readDone = true
		} else {
			s.writeDone = true
		}
		closeNow = s.readDone && s.writeDone
		if closeNow {
			delete(r.streams, id)
		}
	}
	r.mu.Unlock()
	if closeNow {
		s.conn.Close()
	}
}
func (r *relay) pump(id uint32, c net.Conn) {
	r.readers.Add(1)
	go func() {
		defer r.readers.Done()
		b := make([]byte, maxFrame)
		for {
			n, err := c.Read(b)
			if n > 0 {
				if e := r.w.send(data, id, b[:n]); e != nil {
					r.cancel()
					return
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					if r.w.send(endWrite, id, nil) != nil {
						r.cancel()
					}
					r.halfDone(id, true)
				} else {
					r.remove(id)
					if r.w.send(closeStream, id, nil) != nil {
						r.cancel()
					}
				}
				return
			}
		}
	}()
}
func (r *relay) receive() error {
	var lastID uint32
	for {
		p, err := r.w.read()
		if err != nil {
			return err
		}
		if p.id == 0 {
			return errProtocol
		}
		switch p.kind {
		case openStream:
			if r.target == "" || len(p.body) != 0 || p.id <= lastID {
				return errProtocol
			}
			lastID = p.id
			r.mu.Lock()
			full := len(r.streams) >= maxStreams
			r.mu.Unlock()
			if full {
				if err := r.w.send(closeStream, p.id, nil); err != nil {
					return err
				}
				continue
			}
			c, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(r.ctx, "tcp4", r.target)
			if err != nil {
				if err := r.w.send(closeStream, p.id, nil); err != nil {
					return err
				}
				continue
			}
			if !r.add(p.id, c) {
				c.Close()
				if err := r.w.send(closeStream, p.id, nil); err != nil {
					return err
				}
				continue
			}
			r.pump(p.id, c)
		case data, endWrite, closeStream:
			if p.kind != data && len(p.body) != 0 {
				return errProtocol
			}
			r.mu.Lock()
			s := r.streams[p.id]
			r.mu.Unlock()
			if s == nil {
				continue
			} // a close may race in-flight bytes in the other direction
			if p.kind == closeStream {
				r.remove(p.id)
				continue
			}
			if p.kind == endWrite {
				if c, ok := s.conn.(interface{ CloseWrite() error }); ok {
					_ = c.CloseWrite()
				} else {
					r.remove(p.id)
				}
				r.halfDone(p.id, false)
				continue
			}
			// One stalled USB channel cannot retain the entire relay indefinitely.
			_ = s.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
			if err := writeAll(s.conn, p.body); err != nil {
				r.remove(p.id)
				if err := r.w.send(closeStream, p.id, nil); err != nil {
					return err
				}
			}
		default:
			return errProtocol
		}
	}
}
func (r *relay) run(listener net.Listener) error {
	// Closing the dedicated transport unblocks both protocol IO and all pumps.
	done := make(chan struct{})
	go func() {
		select {
		case <-r.ctx.Done():
			r.w.Close()
			if listener != nil {
				listener.Close()
			}
		case <-done:
		}
	}()
	var accepts sync.WaitGroup
	if listener != nil {
		accepts.Add(1)
		go func() {
			defer accepts.Done()
			var id uint32
			for {
				c, err := listener.Accept()
				if err != nil {
					r.cancel()
					return
				}
				id++
				if id == 0 {
					c.Close()
					r.cancel()
					return
				}
				if !r.add(id, c) {
					c.Close()
					continue
				}
				if r.w.send(openStream, id, nil) != nil {
					r.remove(id)
					r.cancel()
					return
				}
				r.pump(id, c)
			}
		}()
	}
	err := r.receive()
	canceled := r.ctx.Err() != nil
	r.cancel()
	r.w.Close()
	if listener != nil {
		listener.Close()
	}
	accepts.Wait() // no new pump can be added after this point
	r.mu.Lock()
	for id, s := range r.streams {
		s.conn.Close()
		delete(r.streams, id)
	}
	r.mu.Unlock()
	r.readers.Wait()
	close(done)
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || canceled {
		return nil
	}
	return err
}

// closeOnCancel also bounds the handshake, before a relay exists.
func closeOnCancel(ctx context.Context, c io.Closer) func() {
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		select {
		case <-ctx.Done():
			c.Close()
		case <-done:
		}
	}()
	return func() { close(done); <-finished }
}

// Export is run beside the physical device. Only a fixed IPv4 loopback exporter
// is reachable; the SSH peer cannot supply additional destinations.
func Export(ctx context.Context, transport io.ReadWriteCloser, port int, c Config, onReady func(int)) error {
	defer transport.Close()
	if err := c.validate(); err != nil {
		return err
	}
	if port < 1024 || port > 65535 {
		return errors.New("export port must be 1024–65535")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.Seconds)*time.Second)
	defer cancel()
	defer closeOnCancel(ctx, transport)()
	handshake, endHandshake := context.WithTimeout(ctx, 15*time.Second)
	stopHandshake := closeOnCancel(handshake, transport)
	defer endHandshake()
	w := &wire{ReadWriteCloser: transport}
	payload, _ := json.Marshal(c)
	if err := w.send(setup, 0, payload); err != nil {
		stopHandshake()
		return err
	}
	p, err := w.read()
	stopHandshake()
	endHandshake()
	if err != nil {
		return fmt.Errorf("remote USB helper did not become ready: %w", err)
	}
	var listenPort int
	if p.kind != ready || p.id != 0 || json.Unmarshal(p.body, &listenPort) != nil || listenPort < 1024 || listenPort > 65535 {
		return errProtocol
	}
	if c.Port != 0 && c.Port != listenPort {
		return errProtocol
	}
	onReady(listenPort)
	return newRelay(ctx, w, net.JoinHostPort("127.0.0.1", strconv.Itoa(port))).run(nil)
}

// Worker runs only as an explicit SSH command on the development host. It binds
// loopback itself, independently of sshd GatewayPorts settings.
func Worker(ctx context.Context, transport io.ReadWriteCloser) error {
	defer transport.Close()
	handshake, cancel := context.WithTimeout(ctx, 10*time.Second)
	stop := closeOnCancel(handshake, transport)
	w := &wire{ReadWriteCloser: transport}
	p, err := w.read()
	stop()
	cancel()
	if err != nil {
		return err
	}
	var c Config
	if p.kind != setup || p.id != 0 || len(p.body) > 256 || json.Unmarshal(p.body, &c) != nil {
		return errProtocol
	}
	if err := c.validate(); err != nil {
		return err
	}
	ctx, cancel = context.WithTimeout(ctx, time.Duration(c.Seconds)*time.Second)
	defer cancel()
	defer closeOnCancel(ctx, transport)()
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.Port)))
	if err != nil {
		return errors.New("cannot bind the remote loopback USB port; choose another port")
	}
	defer listener.Close()
	payload, _ := json.Marshal(listener.Addr().(*net.TCPAddr).Port)
	if err = w.send(ready, 0, payload); err != nil {
		return err
	}
	return newRelay(ctx, w, "").run(listener)
}
