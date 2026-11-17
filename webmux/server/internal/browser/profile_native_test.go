package browser

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSavedProfilePersistsRealChromiumCookies(t *testing.T) {
	if os.Getenv("WEBMUX_REAL_BROWSER") != "1" {
		t.Skip("set WEBMUX_REAL_BROWSER=1 to run installed Chromium")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	newKey := func() string {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			t.Fatal(err)
		}
		key := hex.EncodeToString(raw)
		t.Cleanup(func() {
			root := filepath.Join(home, "webmux-browser-profiles")
			os.RemoveAll(filepath.Join(root, key))
			os.Remove(filepath.Join(root, key+".lock"))
		})
		return key
	}
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/set" {
			http.SetCookie(w, &http.Cookie{Name: "retained", Value: "synthetic", Path: "/", MaxAge: 3600})
		}
		title := "empty"
		if c, err := r.Cookie("retained"); err == nil && c.Value == "synthetic" {
			title = "retained"
		}
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<title>"+title+"</title><p>synthetic profile control</p>")
	}))
	defer site.Close()
	open := func(key string) (func(Request) Frame, func()) {
		t.Helper()
		in, send := io.Pipe()
		receive, out := io.Pipe()
		done := make(chan error, 1)
		go func() { err := Worker(in, out); out.Close(); done <- err }()
		closed := false
		stop := func() {
			if closed {
				return
			}
			closed = true
			send.Close()
			receive.Close()
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(10 * time.Second):
				t.Error("worker did not release profile")
			}
		}
		t.Cleanup(stop)
		decoder := json.NewDecoder(receive)
		call := func(req Request) Frame {
			t.Helper()
			result := make(chan Frame, 1)
			failure := make(chan error, 1)
			go func() {
				if err := json.NewEncoder(send).Encode(req); err != nil {
					failure <- err
					return
				}
				var frame Frame
				if err := decoder.Decode(&frame); err != nil {
					failure <- err
					return
				}
				result <- frame
			}()
			select {
			case frame := <-result:
				if (frame.Error != "" && frame.Error != "Page rendering was interrupted; retry or open the site in your browser") || frame.Protocol != Protocol || frame.ProfileKey != key {
					t.Fatalf("worker %s reply: protocol=%d error=%s", req.Action, frame.Protocol, frame.Error)
				}
				return frame
			case err := <-failure:
				t.Fatal(err)
			case <-time.After(30 * time.Second):
				t.Fatal("worker request timed out")
			}
			return Frame{}
		}
		call(Request{Action: "configure", ProfileKey: key})
		return call, stop
	}
	check := func(call func(Request) Frame, want string) {
		t.Helper()
		call(Request{Action: "navigate", URL: site.URL + "/check"})
		deadline := time.Now().Add(5 * time.Second)
		for {
			frame := call(Request{Action: "frame"})
			for _, tab := range frame.Tabs {
				if tab.ID == frame.Target && tab.Title == want {
					return
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("cookie state did not become %q", want)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	key := newKey()
	first, stop := open(key)
	first(Request{Action: "navigate", URL: site.URL + "/set"})
	check(first, "retained")
	stop()
	second, stopSecond := open(key)
	check(second, "retained")
	stopSecond()
	isolated, stopIsolated := open(newKey())
	check(isolated, "empty")
	stopIsolated()
}
