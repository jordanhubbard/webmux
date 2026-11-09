package recovery

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Allocate a native session before launching the TUI, so concurrent OpenCode
// windows never select one another's "latest" session. The temporary native
// server performs only local session creation; it does not submit a model turn.
func createOpenCodeSession(binary, cwd string) (string, error) {
	return openCodeSession(binary, cwd, "")
}

func openCodeSession(binary, cwd, selectedID string) (string, error) {
	if selectedID != "" && !ValidNativeID("opencode", selectedID) {
		return "", errors.New("invalid OpenCode conversation ID")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	password, err := NewInstance()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(binary, "serve", "--hostname", "127.0.0.1", "--port", strconv.Itoa(port))
	cmd.Dir = cwd
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "OPENCODE_SERVER_PASSWORD=") && !strings.HasPrefix(item, "OPENCODE_SERVER_USERNAME=") {
			cmd.Env = append(cmd.Env, item)
		}
	}
	cmd.Env = append(cmd.Env, "OPENCODE_SERVER_PASSWORD="+password, "OPENCODE_SERVER_USERNAME=webmux")
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	privateProcess(cmd)
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	defer func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
			return
		case <-time.After(time.Second):
		}
		stopPrivateProcess(cmd)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	}()
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	ready := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-done:
			return "", errors.New("OpenCode session preparation exited")
		default:
		}
		req, _ := http.NewRequest("GET", base+"/session?directory="+url.QueryEscape(cwd), nil)
		req.SetBasicAuth("webmux", password)
		response, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		return "", errors.New("OpenCode native session API did not become ready")
	}
	method, endpoint := "POST", "/session"
	if selectedID != "" {
		method, endpoint = "GET", "/session/"+url.PathEscape(selectedID)
	}
	req, _ := http.NewRequest(method, base+endpoint+"?directory="+url.QueryEscape(cwd), bytes.NewBufferString("{}"))
	req.SetBasicAuth("webmux", password)
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var state struct {
		ID        string `json:"id"`
		Directory string `json:"directory"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&state) != nil || !ValidNativeID("opencode", state.ID) || !sameDirectory(state.Directory, cwd) {
		return "", errors.New("OpenCode did not create the requested project session")
	}
	if selectedID != "" && state.ID != selectedID {
		return "", errors.New("OpenCode selected a different native conversation")
	}
	return state.ID, nil
}
