package session

import (
	"encoding/base64"
	"time"
	"unicode/utf8"

	"github.com/jordanhubbard/webmux/server/internal/terminalclipboard"
)

type clipboardRead struct {
	id, viewer, selection string
	expires               time.Time
}

// ClipboardReply admits a single response from the viewer that received the
// query. Binding it to the run prevents delayed clipboard data reaching a new
// process after reconnect. It is terminal protocol input, never broadcast input.
func (b *Broker) ClipboardReply(owner, id, viewerID, requestID, text string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, err := b.ownedLocked(owner, id)
	if err != nil {
		return err
	}
	if b.closed || e.run == nil {
		return ErrClosed
	}
	r := e.run
	query := r.clipboardRead
	if query == nil || query.id != requestID || query.viewer != viewerID || e.focus != viewerID || e.viewers[viewerID] == nil || time.Now().After(query.expires) {
		return ErrNotFound
	}
	if len(text) > terminalclipboard.MaxBytes || !utf8.ValidString(text) {
		return ErrNotFound
	}
	r.clipboardRead = nil
	data := "\x1b]52;" + query.selection + ";" + base64.StdEncoding.EncodeToString([]byte(text)) + "\x1b\\"
	size := int64(len(data))
	if r.inputBytes.Add(size) > 2<<20 {
		r.inputBytes.Add(-size)
		return ErrInputBusy
	}
	select {
	case <-r.stopped:
		r.inputBytes.Add(-size)
		return ErrClosed
	case r.input <- data:
		return nil
	default:
		r.inputBytes.Add(-size)
		return ErrInputBusy
	}
}
