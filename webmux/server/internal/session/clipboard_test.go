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
