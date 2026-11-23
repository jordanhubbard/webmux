package coordinationclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http/httptrace"
	"sync/atomic"
)

// UnavailableError identifies the last observed transport stage without
// printing an endpoint, local socket path, capability, or underlying error.
// Unwrap retains the cause for callers using errors.Is or errors.As.
type UnavailableError struct {
	Stage    string
	TimedOut bool
	Canceled bool
	cause    error
}

func (e *UnavailableError) Error() string {
	reason := "transport failure"
	if e.TimedOut {
		reason = "timeout"
	} else if e.Canceled {
		reason = "canceled"
	}
	return fmt.Sprintf("local coordination service is unavailable (%s; stage=%s)", reason, e.Stage)
}

func (e *UnavailableError) Unwrap() error { return e.cause }

type requestProgress struct{ stage atomic.Value }

func newRequestProgress() *requestProgress {
	p := &requestProgress{}
	p.stage.Store("connection-acquisition")
	return p
}

func (p *requestProgress) trace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		ConnectStart: func(_, _ string) { p.stage.Store("connecting") },
		ConnectDone: func(_, _ string, err error) {
			if err != nil {
				p.stage.Store("connection-failed")
			}
		},
		GotConn: func(httptrace.GotConnInfo) { p.stage.Store("connected") },
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err != nil {
				p.stage.Store("request-write-failed")
			} else {
				p.stage.Store("awaiting-response-headers")
			}
		},
		GotFirstResponseByte: func() { p.stage.Store("response-started") },
	}
}

func (p *requestProgress) failure(err error) error {
	var networkError net.Error
	timedOut := errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout())
	return &UnavailableError{Stage: p.stage.Load().(string), TimedOut: timedOut, Canceled: errors.Is(err, context.Canceled), cause: err}
}
