package session

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/jordanhubbard/webmux/server/internal/config"
)

type transcriptSink interface {
	io.Writer
	io.Closer
}

// A log owns its file and a bounded writer queue. The broker only enqueues;
// writes and closes run without the broker lock. Stop drains accepted output.
type transcriptLog struct {
	path            string
	name            string
	mu              sync.Mutex
	queue           chan string
	done            chan struct{}
	closing         bool
	queuedBytes     int
	reason          string
	failureReported bool // guarded by Broker.mu
}

func (l *transcriptLog) active() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return !l.closing
}
func (l *transcriptLog) append(data string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closing {
		return nil
	}
	if l.queuedBytes+len(data) > 4<<20 {
		return errors.New("transcript writer queue is full")
	}
	select {
	case l.queue <- data:
		l.queuedBytes += len(data)
		return nil
	default:
		return errors.New("transcript writer queue is full")
	}
}
func (l *transcriptLog) stop(reason string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.closing {
		l.closing = true
		l.reason = reason
		close(l.queue)
	}
}

func (b *Broker) writeTranscript(id string, r *run, l *transcriptLog, sink transcriptSink) {
	defer b.workers.Done()
	defer close(l.done)
	var failure error
	for data := range l.queue {
		l.mu.Lock()
		l.queuedBytes -= len(data)
		l.mu.Unlock()
		if failure == nil {
			_, failure = io.WriteString(sink, data)
			if failure != nil {
				l.stop("write_error")
			}
		}
	}
	l.mu.Lock()
	reason := l.reason
	l.mu.Unlock()
	if failure == nil {
		_, failure = io.WriteString(sink, fmt.Sprintf("[webmux transcript stopped %s reason=%s]\r\n", now(), reason))
	}
	failure = errors.Join(failure, sink.Close())
	b.mu.Lock()
	defer b.mu.Unlock()
	if failure != nil {
		b.transcriptErrorLocked(id, r, l, failure)
		reason = "write_error"
	}
	b.audit(map[string]any{"type": "session_transcript_stopped", "session_id": id, "launch_generation": r.generation, "path": l.path, "reason": reason})
}

func safeSessionID(id string) string {
	var name strings.Builder
	for i, unit := range utf16.Encode([]rune(id)) {
		if i >= 80 {
			break
		}
		if unit >= 'a' && unit <= 'z' || unit >= 'A' && unit <= 'Z' || unit >= '0' && unit <= '9' || unit == '_' || unit == '-' {
			name.WriteByte(byte(unit))
		} else {
			name.WriteByte('_')
		}
	}
	if name.Len() == 0 {
		return "session"
	}
	return name.String()
}

func openTranscript(directory, name string, resumed bool) (transcriptSink, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if runtime.GOOS != "windows" {
		if err := root.Chmod(".", 0700); err != nil {
			return nil, err
		}
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if resumed {
		flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
		// Do not follow a substituted symlink or append to a non-regular file.
		if info, err := root.Lstat(name); err == nil && !info.Mode().IsRegular() {
			return nil, errors.New("transcript is not a regular file")
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	file, err := root.OpenFile(name, flags, 0600)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = errors.New("transcript is not a regular file")
	}
	if err == nil && runtime.GOOS != "windows" {
		err = file.Chmod(0600)
	}
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func (b *Broker) loggingEnabledLocked() bool {
	var document config.Document
	if err := b.store.ReadConfig("app.yaml", &document); err != nil {
		b.logger.Warn("read transcript configuration", "error", err)
		return false
	}
	return config.AsObject(document.App["session_logging"])["enabled"] == true
}

func (b *Broker) openTranscriptLocked(e *entry, r *run) *transcriptLog {
	id := safeSessionID(e.value.ID)
	resumed := r.log != nil
	name := ""
	if resumed {
		name = r.log.name
	} else {
		var nonce [4]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			b.logger.Error("transcript filename", "error", err)
			return nil
		}
		name = fmt.Sprintf("session-%s-%s-g%d-%x.log", id, time.Now().UTC().Format("20060102T150405Z"), r.generation, nonce)
	}
	directory := filepath.Join(b.store.Home, "logs", "sessions")
	path := filepath.Join(directory, name)
	sink, err := b.openLog(directory, name, resumed)
	if err != nil {
		b.logger.Warn("open transcript", "session_id", e.value.ID, "error", err)
		b.audit(map[string]any{"type": "session_transcript_error", "session_id": e.value.ID, "launch_generation": r.generation, "path": path, "error": err.Error()})
		return nil
	}
	l := &transcriptLog{path: path, name: name, queue: make(chan string, 256), done: make(chan struct{})}
	action := "started"
	if resumed {
		action = "resumed"
	}
	_ = l.append(fmt.Sprintf("[webmux transcript %s %s session=%s launch=%d]\r\n", action, now(), id, r.generation))
	r.log = l
	b.audit(map[string]any{"type": "session_transcript_started", "session_id": e.value.ID, "launch_generation": r.generation, "path": path, "resumed": resumed})
	b.workers.Add(1)
	go b.writeTranscript(e.value.ID, r, l, sink)
	return l
}

func (b *Broker) transcriptErrorLocked(id string, r *run, l *transcriptLog, err error) {
	l.stop("write_error")
	if l.failureReported {
		return
	}
	l.failureReported = true
	b.logger.Warn("transcript logging failed", "session_id", id, "error", err)
	b.audit(map[string]any{"type": "session_transcript_error", "session_id": id, "launch_generation": r.generation, "path": l.path, "error": err.Error()})
	if e := b.entries[id]; e != nil && e.run == r && r.log == l {
		b.broadcastLocked(e, Event{"type": "transcript_status", "session_id": id, "transcript_enabled": false, "message": "Transcript logging failed"})
	}
}

func (b *Broker) transcriptEventLocked(e *entry) Event {
	event := Event{"type": "transcript_status", "session_id": e.value.ID, "transcript_enabled": e.run != nil && e.run.log.active()}
	if e.run != nil && e.run.log != nil {
		event["transcript_file"] = e.run.log.path
	}
	return event
}

// ToggleTranscript serializes toggles for a session, but waits for disk outside
// the broker lock. A reconnect or deletion can proceed while a log is draining.
func (b *Broker) ToggleTranscript(owner, id, viewerID string) error {
	b.mu.Lock()
	e, err := b.ownedLocked(owner, id)
	b.mu.Unlock()
	if err != nil {
		return err
	}
	e.logOperation.Lock()
	defer e.logOperation.Unlock()
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrClosed
	}
	current, err := b.ownedLocked(owner, id)
	if err != nil || current != e {
		b.mu.Unlock()
		return ErrNotFound
	}
	r := e.run
	if r == nil {
		if v := e.viewers[viewerID]; v != nil {
			v.send(Event{"type": "transcript_status", "session_id": id, "transcript_enabled": false, "message": "Cannot log a disconnected session"})
		}
		b.mu.Unlock()
		return nil
	}
	l := r.log
	pause := l.active()
	if l != nil {
		if pause {
			l.stop("manual_pause")
		}
		b.mu.Unlock()
		<-l.done
		b.mu.Lock()
	}
	if b.closed || b.entries[id] != e || e.run != r {
		b.mu.Unlock()
		return ErrClosed
	}
	if !pause {
		b.openTranscriptLocked(e, r)
	}
	b.broadcastLocked(e, b.transcriptEventLocked(e))
	b.mu.Unlock()
	return nil
}
