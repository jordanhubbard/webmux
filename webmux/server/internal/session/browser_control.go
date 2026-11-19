package session

import (
	"time"

	"github.com/jordanhubbard/webmux/server/internal/recovery"
)

func (b *Broker) publishBrowserRequestLocked(e *entry, requestID, url string, expires time.Time) {
	e.browserRequest = Event{"type": "browser_open", "session_id": e.value.ID, "url": url, "request_id": requestID}
	e.browserExpires = expires
	if viewer := e.viewers[e.focus]; viewer != nil {
		viewer.send(e.browserRequest)
	} else {
		for _, viewer := range e.viewers {
			viewer.send(e.browserRequest)
			break
		}
	}
}

func (b *Broker) applyBrowserControlLocked(e *entry, r *run, pending *recovery.BrowserRequest, observedAt int64) {
	if pending == nil || pending.Expires <= observedAt {
		if e.browserControlID != "" {
			e.browserRequest = nil
			e.browserControlID = ""
		}
		return
	}
	if pending.ID == e.value.LastBrowserAck {
		return
	}
	// The SSH-authenticated response supplies execution-host time. Compare the
	// sealed expiry on that clock, then translate remaining lifetime locally.
	// Ordinary host/controller clock skew must not change the request's TTL.
	hostNow := time.UnixMilli(observedAt)
	url, err := recovery.OpenBrowserRequest(e.value.ID, e.value.RecoveryKey, *pending, hostNow)
	if err != nil {
		r.recoveryError = "The execution host returned an invalid browser request; use the printed sign-in link."
		return
	}
	if e.browserControlID == pending.ID {
		return
	}
	e.browserControlID = pending.ID
	b.publishBrowserRequestLocked(e, pending.ID, url, time.Now().Add(time.UnixMilli(pending.Expires).Sub(hostNow)))
}
