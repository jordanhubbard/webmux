package coordinationclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

// A controllable deadline avoids racing the test runner against a short wall
// clock timeout. HTTP sees a real context deadline after the request is sent.
type controlledDeadline struct{ done <-chan struct{} }

func (c controlledDeadline) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c controlledDeadline) Done() <-chan struct{}       { return c.done }
func (c controlledDeadline) Value(any) any               { return nil }

func (c controlledDeadline) Err() error {
	select {
	case <-c.done:
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func TestUnavailableReportsTransportStageAndPreservesCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprintf("deadline=%v", deadline), func(t *testing.T) {
			arrived, written := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(arrived)
				<-r.Context().Done()
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var requestContext context.Context = ctx
			wantCause := context.Canceled
			if deadline {
				requestContext = controlledDeadline{ctx.Done()}
				wantCause = context.DeadlineExceeded
			}
			requestContext = httptrace.WithClientTrace(requestContext, &httptrace.ClientTrace{
				WroteRequest: func(httptrace.WroteRequestInfo) { close(written) },
			})
			result := make(chan error, 1)
			go func() {
				var out bytes.Buffer
				result <- Request(requestContext, Access{Contract: coordination.Contract, Endpoint: server.URL, Capability: "wmc1.private-test-capability"}, "GET", "/view", nil, &out)
			}()
			for _, signal := range []chan struct{}{arrived, written} {
				select {
				case <-signal:
				case err := <-result:
					t.Fatal("request failed before controlled cancellation", err)
				case <-time.After(10 * time.Second):
					t.Fatal("request did not reach the local server")
				}
			}
			cancel()
			var err error
			select {
			case err = <-result:
			case <-time.After(10 * time.Second):
				t.Fatal("request ignored cancellation")
			}
			var unavailable *UnavailableError
			if !errors.As(err, &unavailable) || !errors.Is(err, wantCause) {
				t.Fatalf("lost classified transport cause: %v", err)
			}
			if unavailable.Stage != "awaiting-response-headers" || unavailable.TimedOut != deadline || unavailable.Canceled == deadline {
				t.Fatal("incorrect stage or cause", err)
			}
			if strings.Contains(err.Error(), server.URL) || strings.Contains(err.Error(), "wmc1.") {
				t.Fatal("transport diagnostic exposed private endpoint or capability")
			}
		})
	}
}

func TestUnavailableRedactsUnderlyingSocketError(t *testing.T) {
	cause := &net.OpError{Op: "dial", Net: "unix", Addr: &net.UnixAddr{Name: "/private/wmc1.secret-session/socket", Net: "unix"}, Err: os.ErrPermission}
	err := newRequestProgress().failure(cause)
	for _, diagnostic := range []string{err.Error(), fmt.Sprintf("%v", err), fmt.Sprintf("%+v", err)} {
		if strings.Contains(diagnostic, "wmc1.") || strings.Contains(diagnostic, "/private/") {
			t.Fatal("diagnostic exposed local capability path")
		}
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatal("lost native error code")
	}
}
