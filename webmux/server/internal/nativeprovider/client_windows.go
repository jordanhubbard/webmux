package nativeprovider

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/screenstate"
	"golang.org/x/sys/windows"
)

func connect(ctx context.Context, i Identity, key, operation string, cols, rows int, renderer string) (net.Conn, frame, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	c, err := (&net.Dialer{}).DialContext(ctx, "unix", i.Endpoint)
	if err != nil {
		return nil, frame{}, err
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
	request := frame{Protocol: 1, Kind: operation, Session: i.Session, Instance: i.Instance, Key: key, Cols: cols, Rows: rows}
	if operation == "attach" {
		switch i.Replay {
		case "screen-v3":
			request.Renderer = renderer
		case "screen-v2":
			request.Renderer = renderer
			if renderer == screenstate.XtermUnderlineRendererProfile {
				request.Renderer = screenstate.XtermRendererProfile
			}
			// Strict screen-v1 owners predate the renderer field. Omit it entirely.
		}
	}
	if err := writeFrame(c, request); err != nil {
		return nil, frame{}, err
	}
	var reply frame
	if err := readFrame(c, &reply); err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return nil, frame{}, err
	}
	if reply.Error != "" {
		return nil, frame{}, errors.New(reply.Error)
	}
	if reply.Identity == nil || reply.Identity.Instance != i.Instance || reply.Identity.Session != i.Session || reply.Identity.OwnerPID != i.OwnerPID {
		return nil, frame{}, errors.New("native provider identity changed")
	}
	want := "ready"
	if operation == "attach" {
		want = "attached"
	}
	if operation == "stop" {
		want = "stopping"
	}
	if reply.Kind != want {
		return nil, frame{}, errors.New("invalid native provider response")
	}
	if operation == "attach" && reply.Identity.Replay != "screen-v1" && reply.Identity.Replay != "screen-v2" && reply.Identity.Replay != "screen-v3" {
		return nil, frame{}, errors.New("native provider cannot restore a complete terminal view; keep the existing job and recover manually")
	}
	if operation == "attach" && reply.Identity.Replay != i.Replay {
		return nil, frame{}, errors.New("native provider replay capability changed")
	}
	if reply.RequiredRenderer != "" {
		if request.Renderer == "" || !screenstate.RendererSupports(request.Renderer, reply.RequiredRenderer) || (reply.Identity.Replay != "screen-v2" && reply.Identity.Replay != "screen-v3") || (reply.RequiredRenderer == screenstate.XtermUnderlineRendererProfile && reply.Identity.Replay != "screen-v3") || len(reply.RendererToken) != 32 {
			return nil, frame{}, errors.New("unnegotiated native snapshot profile")
		}
		if decoded, err := hex.DecodeString(reply.RendererToken); err != nil || hex.EncodeToString(decoded) != reply.RendererToken {
			return nil, frame{}, errors.New("invalid native snapshot token")
		}
	} else if reply.RendererToken != "" {
		return nil, frame{}, errors.New("unexpected native snapshot token")
	}
	// Attach keeps the connection after this handshake context ends. Join or
	// stop its callback before clearing the deadline for subsequent streaming.
	stopContext()
	if ctx.Err() != nil {
		return nil, frame{}, ctx.Err()
	}
	_ = c.SetDeadline(time.Time{})
	failed = false
	return c, reply, nil
}

func Probe(ctx context.Context, i Identity, key string) (Identity, error) {
	c, result, err := connect(ctx, i, key, "probe", 0, 0, "")
	if c != nil {
		_ = c.Close()
	}
	if err != nil {
		return Identity{}, err
	}
	return *result.Identity, nil
}
func Stop(ctx context.Context, i Identity, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if i.OwnerPID <= 0 {
		return errors.New("invalid native provider owner")
	}
	// Retain the original process object before dispatch. A later PID reuse
	// must not make us wait on, or report the state of, another process.
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(i.OwnerPID))
	if err != nil {
		return errors.New("native provider owner exit cannot be observed; stop was not dispatched")
	}
	defer windows.CloseHandle(handle)
	c, _, err := connect(ctx, i, key, "stop", 0, 0, "")
	if c != nil {
		_ = c.Close()
	}
	if err != nil {
		return err
	}
	// "stopping" acknowledges the authenticated request before the owner
	// closes its terminal/job. It is not proof that teardown has completed.
	for {
		if err := ctx.Err(); err != nil {
			return errors.Join(errors.New("native provider stop acknowledged; owner exit remains unconfirmed"), err)
		}
		status, err := windows.WaitForSingleObject(handle, 25)
		if err != nil {
			return errors.New("native provider stop acknowledged; owner exit could not be observed")
		}
		if status == windows.WAIT_OBJECT_0 {
			return nil
		}
		if status != uint32(windows.WAIT_TIMEOUT) {
			return errors.New("native provider stop acknowledged; owner exit remains unconfirmed")
		}
	}
}

type Client struct {
	requiredRenderer string
	rendererToken    string
	connection       net.Conn
	reader           *io.PipeReader
	writer           *io.PipeWriter
	writeMu          sync.Mutex
	done             chan struct{}
	code             int
	err              error
	once             sync.Once
}

func Attach(ctx context.Context, i Identity, key string, cols, rows int) (*Client, error) {
	return AttachWithProfile(ctx, i, key, cols, rows, "")
}

// AttachWithProfile negotiates extensions before receiving any snapshot output.
func AttachWithProfile(ctx context.Context, i Identity, key string, cols, rows int, renderer string) (*Client, error) {
	if !screenstate.RendererProfileKnown(renderer) {
		return nil, errors.New("unknown native snapshot profile")
	}
	if !validSize(cols, rows) {
		return nil, errors.New("invalid native provider terminal size")
	}
	c, reply, err := connect(ctx, i, key, "attach", cols, rows, renderer)
	if err != nil {
		return nil, err
	}
	r, w := io.Pipe()
	client := &Client{requiredRenderer: reply.RequiredRenderer, rendererToken: reply.RendererToken, connection: c, reader: r, writer: w, done: make(chan struct{}), code: -1}
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

// SnapshotProfile is immutable handshake metadata, never parsed from job output.
func (c *Client) SnapshotProfile() (string, string) { return c.requiredRenderer, c.rendererToken }
