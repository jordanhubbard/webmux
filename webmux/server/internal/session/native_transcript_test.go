package session

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeManagedTranscriptRetainsPathAcrossTwoAttachmentLosses(t *testing.T) {
	b, _, value, _ := nativeRigFixture(t)
	b.mu.Lock()
	original := b.entries[value.ID].run.log.path
	b.mu.Unlock()
	for cycle := range 2 {
		b.mu.Lock()
		p := b.entries[value.ID].run.process.(*fakeProcess)
		b.mu.Unlock()
		before := fmt.Sprintf("native-before-loss-%d", cycle)
		p.output(t, before+"\n")
		logContains(t, original, before)
		p.exit()
		eventually(t, func() bool {
			b.mu.Lock()
			defer b.mu.Unlock()
			return b.entries[value.ID].run == nil
		})
		prefix := []byte(logContents(t, original))
		if _, err := b.ReconnectManaged("owner", value.ID, *value.Rig, []string{"same-native-original"}); err != nil {
			t.Fatal(err)
		}
		b.mu.Lock()
		r := b.entries[value.ID].run
		path, next := r.log.path, r.process.(*fakeProcess)
		b.mu.Unlock()
		if path != original {
			t.Fatal("native reattachment changed retained transcript path", original, path)
		}
		after := fmt.Sprintf("native-after-loss-%d", cycle)
		next.output(t, after+"\n")
		logContains(t, original, after)
		if !bytes.HasPrefix([]byte(logContents(t, original)), prefix) {
			t.Fatal("native reattachment replaced transcript prefix")
		}
	}
}

func TestNativeManagedTranscriptPauseSurvivesAttachmentLoss(t *testing.T) {
	b, _, value, _ := nativeRigFixture(t)
	b.mu.Lock()
	original := b.entries[value.ID].run.log.path
	p := b.entries[value.ID].run.process.(*fakeProcess)
	b.mu.Unlock()
	if err := b.ToggleTranscript("owner", value.ID, ""); err != nil {
		t.Fatal(err)
	}
	prefix := logContents(t, original)
	p.exit()
	eventually(t, func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		return b.entries[value.ID].run == nil
	})
	if _, err := b.ReconnectManaged("owner", value.ID, *value.Rig, []string{"same-native-original"}); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	r := b.entries[value.ID].run
	next, active := r.process.(*fakeProcess), r.log.active()
	b.mu.Unlock()
	if active || logContents(t, original) != prefix {
		t.Fatal("native recovery resumed paused logging without approval")
	}
	if err := b.ToggleTranscript("owner", value.ID, ""); err != nil {
		t.Fatal(err)
	}
	next.output(t, "approved-native-transcript-resume\n")
	logContains(t, original, "approved-native-transcript-resume")
}

func TestNativeManagedTranscriptNeverAppendsToForeignPersistedName(t *testing.T) {
	b, store, value, _ := nativeRigFixture(t)
	foreign := filepath.Join(store.Home, "logs", "sessions", "session-other-owner.log")
	if err := os.WriteFile(foreign, []byte("foreign-original"), 0600); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	b.entries[value.ID].value.Recovery.Transcripts = []string{filepath.Base(foreign)}
	b.mu.Unlock()
	if _, err := b.ReconnectManaged("owner", value.ID, *value.Rig, []string{"same-native-original"}); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	r := b.entries[value.ID].run
	path, next := r.log.path, r.process.(*fakeProcess)
	b.mu.Unlock()
	if path == foreign {
		t.Fatal("native recovery accepted another session's log")
	}
	next.output(t, "native-new-output\n")
	logContains(t, path, "native-new-output")
	if logContents(t, foreign) != "foreign-original" {
		t.Fatal("native output changed another session's log")
	}
}

func TestNativeManagedTranscriptRestoresOriginalPathAfterControllerRestart(t *testing.T) {
	for _, remove := range []bool{false, true} {
		t.Run(fmt.Sprintf("removed=%v", remove), func(t *testing.T) {
			b, store, value, _ := nativeRigFixture(t)
			b.mu.Lock()
			original := b.entries[value.ID].run.log.path
			p := b.entries[value.ID].run.process.(*fakeProcess)
			b.mu.Unlock()
			p.output(t, "before-controller-restart\n")
			logContains(t, original, "before-controller-restart")
			if err := b.Close(); err != nil {
				t.Fatal(err)
			}
			prefix := []byte(logContents(t, original))
			if remove {
				if err := os.Remove(original); err != nil {
					t.Fatal(err)
				}
			}
			restored, err := newBroker(store, nil, b.launch)
			if err != nil {
				t.Fatal(err)
			}
			defer restored.Close()
			if _, err := restored.ReconnectManaged("owner", value.ID, *value.Rig, []string{"same-native-original"}); err != nil {
				t.Fatal(err)
			}
			restored.mu.Lock()
			r := restored.entries[value.ID].run
			path, next := r.log.path, r.process.(*fakeProcess)
			restored.mu.Unlock()
			if path != original {
				t.Fatal("controller restart changed transcript path", original, path)
			}
			next.output(t, "after-controller-restart\n")
			logContains(t, original, "after-controller-restart")
			if !remove && !bytes.HasPrefix([]byte(logContents(t, original)), prefix) {
				t.Fatal("controller restart replaced transcript prefix")
			}
		})
	}
}
