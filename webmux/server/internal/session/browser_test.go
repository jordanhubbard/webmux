package session

import (
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/browseropen"
)

func TestBrowserRequestOwnershipLoggingAndAcknowledgement(t *testing.T) {
	b, _, launched := fixture(t)
	s := create(t, b, "owner")
	p := <-launched
	p.output(t, "ready\n")
	eventually(t, func() bool { text, _ := b.Scrollback("owner", s.ID); return text != "" })
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
	url := "https://login.example/authorize?state=private-browser-request"
	packet := browseropen.Packet(url)
	p.output(t, packet[:9])
	p.output(t, packet[9:]+"normal-output\n")
	var request Event
	for request == nil {
		select {
		case <-v.Ready():
		case <-time.After(5 * time.Second):
			t.Fatal("browser request was not delivered")
		}
		value := v.Take()
		if value["type"] == "browser_open" {
			request = value
		}
	}
	if request["url"] != url {
		t.Fatalf("wrong launch: %v", request)
	}
	requestID := request["request_id"].(string)
	b.AcknowledgeBrowser("foreign", s.ID, requestID)
	b.mu.Lock()
	pending := b.entries[s.ID].browserRequest
	b.mu.Unlock()
	if pending == nil {
		t.Fatal("foreign owner cleared browser request")
	}
	b.AcknowledgeBrowser("owner", s.ID, requestID)
	text, _ := b.Scrollback("owner", s.ID)
	if strings.Contains(text, "private-browser-request") || strings.Contains(text, browseropen.Prefix) {
		t.Fatal("browser request entered scrollback")
	}
	b.mu.Lock()
	pending = b.entries[s.ID].browserRequest
	b.mu.Unlock()
	if pending != nil {
		t.Fatal("acknowledged browser request remains pending")
	}
	if err := b.ToggleTranscript("owner", s.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	contents := logContents(t, path)
	if strings.Contains(contents, "private-browser-request") || strings.Contains(contents, browseropen.Prefix) || strings.Contains(contents, packet[9:20]) {
		t.Fatal("browser request entered transcript")
	}
	if !strings.Contains(contents, "normal-output") {
		t.Fatal("ordinary output was lost")
	}
}

func TestBrowserRequestSurvivesViewerReconnectButNotAcknowledgement(t *testing.T) {
	b, _, launched := fixture(t)
	s := create(t, b, "owner")
	p := <-launched
	p.output(t, browseropen.Packet("https://login.example/pending"))
	eventually(t, func() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.entries[s.ID].browserRequest != nil })
	v, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	event(t, v, "viewer_join")
	event(t, v, "status")
	request := event(t, v, "browser_open")
	b.AcknowledgeBrowser("owner", s.ID, request["request_id"].(string))
	b.Leave("owner", s.ID, v.ID)
	v, err = b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	event(t, v, "viewer_join")
	event(t, v, "status")
	select {
	case <-v.Ready():
		t.Fatalf("acknowledged request replayed: %v", v.Take())
	default:
	}
}
