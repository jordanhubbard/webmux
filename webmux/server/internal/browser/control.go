package browser

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"regexp"
	"sync"
	"time"
)

const ControlDuration = 30 * time.Second

var ErrControl = errors.New("This viewer does not control the browser. Take control here before changing it.")

func ValidViewer(value string) bool { return viewerSyntax.MatchString(value) }

var viewerSyntax = regexp.MustCompile(`^[a-f0-9]{32}$`)

type Control struct {
	Owned     bool   `json:"owned"`
	Available bool   `json:"available"`
	ExpiresAt string `json:"expiresAt,omitempty"`
	Token     string `json:"token,omitempty"`
}

type controllerLease struct {
	mu            sync.Mutex
	viewer, token string
	until         time.Time
	now           func() time.Time
}

func (l *controllerLease) currentTime() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}
func (l *controllerLease) owns(viewer, token string, now time.Time) bool {
	return now.Before(l.until) && viewer == l.viewer && token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(l.token)) == 1
}
func (l *controllerLease) control(viewer, token string, claim, force, release bool) (Control, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if (claim || release) && !viewerSyntax.MatchString(viewer) {
		return Control{}, errors.New("Reload WebMux to identify this browser viewer")
	}
	now := l.currentTime()
	owned := l.owns(viewer, token, now)
	if release {
		if !owned {
			return Control{}, ErrControl
		}
		l.viewer, l.token = "", ""
		l.until = time.Time{}
	} else if claim && (!now.Before(l.until) || force) {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return Control{}, err
		}
		l.viewer, l.token, l.until = viewer, hex.EncodeToString(raw), now.Add(ControlDuration)
		owned = true
	} else if owned {
		l.until = now.Add(ControlDuration)
	}
	state := Control{Owned: owned && !release, Available: !now.Before(l.until)}
	if now.Before(l.until) {
		state.ExpiresAt = l.until.UTC().Format(time.RFC3339Nano)
	}
	if state.Owned {
		state.Token = l.token
	}
	return state, nil
}
func (l *controllerLease) authorize(viewer, token string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.currentTime()
	if !l.owns(viewer, token, now) {
		return ErrControl
	}
	l.until = now.Add(ControlDuration)
	return nil
}

func (m *Manager) Control(id, viewer, token string, claim, force, release bool) (Control, error) {
	m.mu.Lock()
	c := m.clients[id]
	m.mu.Unlock()
	if c == nil {
		return Control{}, errors.New("Browser has ended; reopen it")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	before, _ := c.control.control(viewer, token, false, false, false)
	state, err := c.control.control(viewer, token, claim, force, release)
	if err == nil && (release || (state.Owned && state.Token != before.Token)) {
		c.resetPending = true
	}
	return state, err
}
