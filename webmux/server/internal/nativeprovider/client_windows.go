package nativeprovider

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

func connect(ctx context.Context, i Identity, key, operation string, cols, rows int) (net.Conn, Identity, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	c, err := (&net.Dialer{}).DialContext(ctx, "unix", i.Endpoint)
	if err != nil {
		return nil, Identity{}, err
	}
	failed := true
	defer func() {
		if failed {
			_ = c.Close()
		}
	}()
	deadline, _ := ctx.Deadline()
	_ = c.SetDeadline(deadline)
	cancelled := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = c.SetDeadline(time.Now()); close(cancelled) })
	var once sync.Once
	stopContext := func() {
		once.Do(func() {
			if !stop() {
				<-cancelled
			}
		})
	}
	defer stopContext()
	if err := writeFrame(c, frame{Protocol: 1, Kind: operation, Session: i.Session, Instance: i.Instance, Key: key, Cols: cols, Rows: rows}); err != nil {
		return nil, Identity{}, err
	}
	var reply frame
	if err := readFrame(c, &reply); err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return nil, Identity{}, err
	}
	if reply.Error != "" {
		return nil, Identity{}, errors.New(reply.Error)
	}
	if reply.Identity == nil || reply.Identity.Instance != i.Instance || reply.Identity.Session != i.Session || reply.Identity.OwnerPID != i.OwnerPID {
		return nil, Identity{}, errors.New("native provider identity changed")
	}
	want := "ready"
	if operation == "attach" {
		want = "attached"
	}
	if operation == "stop" {
		want = "stopping"
	}
	if reply.Kind != want {
		return nil, Identity{}, errors.New("invalid native provider response")
	}
	// Attach keeps the connection after this handshake context ends. Join or
	// stop its callback before clearing the deadline for subsequent streaming.
	stopContext()
	if ctx.Err() != nil {
		return nil, Identity{}, ctx.Err()
	}
	_ = c.SetDeadline(time.Time{})
	failed = false
	return c, *reply.Identity, nil
}

func Probe(ctx context.Context, i Identity, key string) (Identity, error) {
	c, result, err := connect(ctx, i, key, "probe", 0, 0)
	if c != nil {
		_ = c.Close()
	}
	return result, err
}
func Stop(ctx context.Context, i Identity, key string) error {
	c, _, err := connect(ctx, i, key, "stop", 0, 0)
	if c != nil {
		_ = c.Close()
	}
	return err
}

type Client struct {
	connection net.Conn
	reader     *io.PipeReader
	writer     *io.PipeWriter
	writeMu    sync.Mutex
	done       chan struct{}
	code       int
	err        error
	once       sync.Once
}

func Attach(ctx context.Context, i Identity, key string, cols, rows int) (*Client, error) {
	if !validSize(cols, rows) {
		return nil, errors.New("invalid native provider terminal size")
	}
	c, _, err := connect(ctx, i, key, "attach", cols, rows)
	if err != nil {
		return nil, err
	}
	r, w := io.Pipe()
	client := &Client{connection: c, reader: r, writer: w, done: make(chan struct{}), code: -1}
	go func() {
		defer close(client.done)
		defer c.Close()
		defer w.Close()
		for {
			var message frame
			if err := readFrame(c, &message); err != nil {
				client.err = io.ErrUnexpectedEOF
				return
			}
			switch message.Kind {
			case "data":
				if len(message.Data) > maxData {
					client.err = errors.New("native provider output exceeded its bound")
					return
				}
				if _, err := w.Write(message.Data); err != nil {
					client.err = err
					return
				}
			case "exit":
				client.code = message.Code
				if message.Error != "" {
					client.err = errors.New(message.Error)
				}
				return
			default:
				client.err = errors.New("invalid native provider stream")
				return
			}
		}
	}()
	return client, nil
}

func (c *Client) Read(p []byte) (int, error) { return c.reader.Read(p) }
func (c *Client) send(f frame) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.connection.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return writeFrame(c.connection, f)
}
func (c *Client) Write(p []byte) (int, error) {
	n := 0
	for len(p) > 0 {
		k := min(len(p), maxData)
		if err := c.send(frame{Kind: "input", Data: p[:k]}); err != nil {
			return n, err
		}
		n += k
		p = p[k:]
	}
	return n, nil
}
func (c *Client) Resize(cols, rows int) error {
	if !validSize(cols, rows) {
		return errors.New("invalid native provider terminal size")
	}
	return c.send(frame{Kind: "resize", Cols: cols, Rows: rows})
}
func (c *Client) Wait() (int, error) { <-c.done; return c.code, c.err }
func (c *Client) Close() error {
	var err error
	c.once.Do(func() { _ = c.reader.Close(); err = c.connection.Close() })
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}
