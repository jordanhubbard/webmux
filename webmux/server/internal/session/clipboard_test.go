package session

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestClipboardWritesAreLiveOnlyAndNeverLogged(t *testing.T) {
	b, _, launched := fixture(t)
	s := create(t, b, "owner")
	p := <-launched
	p.output(t, "ready\n")
	eventually(t, func() bool { value, _ := b.Scrollback("owner", s.ID); return value != "" })
	v, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	event(t, v, "viewer_join")
	event(t, v, "status")
	event(t, v, "output")
	if err := b.ToggleTranscript("owner", s.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	path := event(t, v, "transcript_status")["transcript_file"].(string)
	text := "private desktop clipboard"
	packet := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\a"
	p.output(t, packet[:9])
	p.output(t, packet[9:]+"visible-output\n")
	var copied Event
	for copied == nil {
		select {
		case <-v.Ready():
		case <-time.After(5 * time.Second):
			t.Fatal("clipboard event was not delivered")
		}
		value := v.Take()
		if value["type"] == "clipboard" {
			copied = value
		}
	}
	if got := copied["data"]; got != text {
		t.Fatalf("wrong clipboard: %v", got)
	}
	event(t, v, "output")
	scrollback, _ := b.Scrollback("owner", s.ID)
	if strings.Contains(scrollback, "52;") || strings.Contains(scrollback, base64.StdEncoding.EncodeToString([]byte(text))) {
		t.Fatal("clipboard was retained in scrollback")
	}
	if err := b.ToggleTranscript("owner", s.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	contents := logContents(t, path)
	if strings.Contains(contents, "52;") || strings.Contains(contents, base64.StdEncoding.EncodeToString([]byte(text))) {
		t.Fatal("clipboard was logged")
	}
	b.Leave("owner", s.ID, v.ID)
	rejoined, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	event(t, rejoined, "viewer_join")
	event(t, rejoined, "status")
	if got := event(t, rejoined, "output")["data"].(string); !strings.Contains(got, "visible-output") {
		t.Fatal("ordinary output lost on replay")
	}
	if value := rejoined.Take(); value != nil && value["type"] == "clipboard" {
		t.Fatal("clipboard was replayed")
	}
}

func TestClipboardReadsBelongToOneViewerAndProvider(t *testing.T) {
	b, _, launched := fixture(t)
	s := create(t, b, "owner")
	p := <-launched
	v, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	query := func() string {
		t.Helper()
		p.output(t, "\x1b]52;c;?\a")
		for {
			select {
			case <-v.Ready():
				message := v.Take()
				if message["type"] == "clipboard_read" {
					return message["request_id"].(string)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("clipboard query not delivered")
				return ""
			}
		}
	}
	noInput := func() {
		t.Helper()
		select {
		case data := <-p.input:
			t.Fatalf("unexpected protocol input: %q", data)
		case <-time.After(20 * time.Millisecond):
		}
	}
	id := query()
	if err := b.ClipboardReply("other-owner", s.ID, v.ID, id, "wrong owner"); err == nil {
		t.Fatal("accepted another owner")
	}
	if err := b.ClipboardReply("owner", s.ID, other.ID, id, "wrong viewer"); err == nil {
		t.Fatal("accepted another viewer")
	}
	if err := b.ClipboardReply("owner", s.ID, v.ID, "invented", "unsolicited"); err == nil {
		t.Fatal("accepted unsolicited data")
	}
	if err := b.ClipboardReply("owner", s.ID, v.ID, id, strings.Repeat("x", 128*1024+1)); err == nil {
		t.Fatal("accepted oversized data")
	}
	noInput()
	text := "desktop 😀\ntext"
	if err := b.ClipboardReply("owner", s.ID, v.ID, id, text); err != nil {
		t.Fatal(err)
	}
	select {
	case data := <-p.input:
		if want := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\x1b\\"; data != want {
			t.Fatalf("got %q, want %q", data, want)
		}
	case <-time.After(time.Second):
		t.Fatal("clipboard response never reached process")
	}
	if err := b.ClipboardReply("owner", s.ID, v.ID, id, "replayed"); err == nil {
		t.Fatal("accepted duplicate")
	}
	noInput()
	id = query()
	if err := b.Focus("owner", s.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	if err := b.Focus("owner", s.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	if err := b.ClipboardReply("owner", s.ID, v.ID, id, "old focus"); err == nil {
		t.Fatal("accepted stale focus")
	}
	noInput()
	id = query()
	b.mu.Lock()
	b.entries[s.ID].run.clipboardRead.expires = time.Now().Add(-time.Second)
	b.mu.Unlock()
	if err := b.ClipboardReply("owner", s.ID, v.ID, id, "expired"); err == nil {
		t.Fatal("accepted expired response")
	}
	noInput()
	id = query()
	if _, err := b.Reconnect("owner", s.ID, ""); err != nil {
		t.Fatal(err)
	}
	next := <-launched
	if err := b.ClipboardReply("owner", s.ID, v.ID, id, "old process"); err == nil {
		t.Fatal("accepted previous process response")
	}
	select {
	case data := <-next.input:
		t.Fatalf("clipboard reached replacement process: %q", data)
	case <-time.After(20 * time.Millisecond):
	}
}
