package session

import (
	"errors"
	"strings"
	"testing"

	"github.com/jordanhubbard/webmux/server/internal/screenstate"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
)

type profileProcess struct {
	*fakeProcess
	profile, token string
}

func (p *profileProcess) SnapshotProfile() (string, string) { return p.profile, p.token }

func TestSnapshotProfilePrecedesReplayAndPreservesJob(t *testing.T) {
	for _, required := range []string{screenstate.XtermRendererProfile, screenstate.XtermUnderlineRendererProfile} {
		t.Run(required, func(t *testing.T) {

			b, _, launched := fixture(t)
			original := b.launch
			token := strings.Repeat("a", 32)
			b.launch = func(r terminal.LaunchRequest, password string) (process, error) {
				p, err := original(r, password)
				if err != nil {
					return nil, err
				}
				return &profileProcess{p.(*fakeProcess), required, token}, nil
			}
			s := create(t, b, "owner")
			p := <-launched
			incompatible := []string{"", "unknown"}
			if required == screenstate.XtermUnderlineRendererProfile {
				incompatible = append(incompatible, screenstate.XtermRendererProfile)
			}
			for _, profile := range incompatible {
				if v, err := b.JoinWithProfile("owner", s.ID, profile); v != nil || !errors.Is(err, ErrRendererUnsupported) {
					t.Fatal("incompatible viewer admitted", err)
				}
			}
			if _, err := b.JoinWithProfile("foreign", s.ID, screenstate.XtermRendererProfile); !errors.Is(err, ErrNotFound) {
				t.Fatal("capability bypassed ownership", err)
			}
			first, err := b.JoinWithProfile("owner", s.ID, screenstate.XtermUnderlineRendererProfile)
			if err != nil {
				t.Fatal(err)
			}
			event(t, first, "viewer_join")
			checkStatus := func(v *Viewer) {
				t.Helper()
				status := event(t, v, "status")
				if status["snapshot_profile"] != required || status["snapshot_token"] != token {
					t.Fatal("metadata missing before bytes", status)
				}
			}
			checkStatus(first)
			packet := "\x1b]7777;webmux.replay.v1;" + token + ";Wg==\a"
			if required == screenstate.XtermUnderlineRendererProfile {
				packet = "\x1b]7777;webmux.replay.v2;" + token + ";eyJ1IjoyLCJyIjoiWiJ9\a"
			}
			// Existing recovery, clipboard and browser filters must preserve this OSC,
			// including arbitrary native stream boundaries, without creating host input.
			for _, c := range []byte(packet) {
				p.output(t, string(c))
			}
			checkStatus(first)
			var got string
			for len(got) < len(packet) {
				got += event(t, first, "output")["data"].(string)
			}
			if got != packet {
				t.Fatalf("profile packet changed in broker: %q", got)
			}
			second, err := b.JoinWithProfile("owner", s.ID, required)
			if err != nil {
				t.Fatal(err)
			}
			event(t, second, "viewer_join")
			checkStatus(second)
			if data := event(t, second, "output")["data"]; data != packet {
				t.Fatal("later viewer lost replay", data)
			}
			b.Leave("owner", s.ID, first.ID)
			b.Leave("owner", s.ID, second.ID)
			select {
			case <-p.done:
				t.Fatal("viewer detach stopped job")
			default:
			}
			select {
			case data := <-p.input:
				t.Fatalf("snapshot generated terminal input: %q", data)
			default:
			}

		})
	}
}

func TestSnapshotProfileTransitionClosesOnlyIncompatibleViewers(t *testing.T) {
	b, _, launched := fixture(t)
	s := create(t, b, "owner")
	p := <-launched
	old, err := b.Join("owner", s.ID)
	if err != nil {
		t.Fatal(err)
	}
	modern, err := b.JoinWithProfile("owner", s.ID, screenstate.XtermRendererProfile)
	if err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	e := b.entries[s.ID]
	b.setSnapshotProfileLocked(e, screenstate.XtermRendererProfile, strings.Repeat("a", 32))
	remaining := len(e.viewers)
	b.mu.Unlock()
	if code, _ := old.CloseReason(); code != 1008 {
		t.Fatal("legacy viewer not refused", code)
	}
	select {
	case <-modern.Done():
		t.Fatal("compatible viewer was closed")
	default:
	}
	select {
	case <-p.done:
		t.Fatal("capability transition stopped owned process")
	default:
	}
	if remaining != 1 {
		t.Fatal("incompatible viewer retained", remaining)
	}
}

func TestV2RequirementDetachesV1ViewerWithoutStoppingJob(t *testing.T) {
	b, _, launched := fixture(t)
	s := create(t, b, "owner")
	p := <-launched
	old, err := b.JoinWithProfile("owner", s.ID, screenstate.XtermRendererProfile)
	if err != nil {
		t.Fatal(err)
	}
	modern, err := b.JoinWithProfile("owner", s.ID, screenstate.XtermUnderlineRendererProfile)
	if err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	b.setSnapshotProfileLocked(b.entries[s.ID], screenstate.XtermRendererProfile, strings.Repeat("a", 32))
	b.mu.Unlock()
	select {
	case <-old.Done():
		t.Fatal("V1 viewer refused V1 requirement")
	default:
	}
	select {
	case <-modern.Done():
		t.Fatal("V2 viewer refused V1 requirement")
	default:
	}
	b.mu.Lock()
	b.setSnapshotProfileLocked(b.entries[s.ID], screenstate.XtermUnderlineRendererProfile, strings.Repeat("b", 32))
	count := len(b.entries[s.ID].viewers)
	b.mu.Unlock()
	if code, _ := old.CloseReason(); code != 1008 {
		t.Fatal("V1 viewer not detached", code)
	}
	select {
	case <-modern.Done():
		t.Fatal("V2 viewer detached")
	default:
	}
	select {
	case <-p.done:
		t.Fatal("owned job stopped")
	default:
	}
	if count != 1 {
		t.Fatal("incompatible viewer retained", count)
	}
}
