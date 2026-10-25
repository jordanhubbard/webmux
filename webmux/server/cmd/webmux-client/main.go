// Command webmux-client connects a user-managed host to a WebMux server.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

const protocol = 1

type identity struct {
	Server     string `json:"server"`
	ID         string `json:"id"`
	Credential string `json:"credential"`
}

type snapshot struct {
	Hostname string   `json:"hostname"`
	OS       string   `json:"os"`
	Arch     string   `json:"arch"`
	CPUs     int      `json:"cpus"`
	Load1    *float64 `json:"load1,omitempty"`
}

type frame struct {
	Type     string    `json:"type"`
	Protocol int       `json:"protocol,omitempty"`
	ID       string    `json:"id,omitempty"`
	Command  string    `json:"command,omitempty"`
	Snapshot *snapshot `json:"snapshot,omitempty"`
}

var errRevoked = errors.New("client credential was revoked")

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "webmux-client:", err)
		os.Exit(1)
	}
}

func run() error {
	flags := flag.NewFlagSet("webmux-client", flag.ContinueOnError)
	server := flags.String("server", "", "WebMux server address or URL, for example webmux.example:8443")
	statePath := flags.String("state", "", "private client identity file (optional)")
	background := flags.Bool("background", false, "continue in the background after enrollment")
	pairCode := flags.Bool("pair-code", false, "print a new one-time host pairing code")
	allowHTTP := flags.Bool("allow-insecure-http", false, "allow cleartext HTTP for an explicitly trusted network")
	child := flags.Bool("run-child", false, "internal background worker")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if *server == "" && flags.NArg() == 1 {
		*server = flags.Arg(0)
	}
	endpoint, err := normalizeServer(*server, *allowHTTP)
	if err != nil {
		return err
	}
	if *statePath == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		key := sha256.Sum256([]byte(endpoint))
		*statePath = filepath.Join(configDir, "webmux-client", hex.EncodeToString(key[:12])+".json")
	}
	id, err := loadIdentity(*statePath)
	if errors.Is(err, os.ErrNotExist) && !*child {
		id, err = enroll(endpoint)
		if err != nil {
			return err
		}
		if err := saveIdentity(*statePath, id); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if id.Server != endpoint {
		return errors.New("identity belongs to a different WebMux server")
	}
	if *pairCode {
		return printPairCode(endpoint, id.Credential)
	}
	if *background && !*child {
		if err := probeConnection(endpoint, id.Credential); err != nil {
			return fmt.Errorf("server check-in failed: %w", err)
		}
		return startBackground(endpoint, *statePath, *allowHTTP)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintf(os.Stderr, "Connected host identity: %s\n", id.ID)
	backoff := time.Second
	for ctx.Err() == nil {
		err := connect(ctx, endpoint, id.Credential)
		if errors.Is(err, errRevoked) {
			return err
		}
		if ctx.Err() != nil {
			break
		}
		fmt.Fprintf(os.Stderr, "connection lost: %v; retrying in %s\n", err, backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
	return nil
}

func normalizeServer(raw string, allowHTTP bool) (string, error) {
	if raw != "" && !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	address, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil || address.Host == "" || (address.Scheme != "https" && address.Scheme != "http") || address.User != nil || address.RawQuery != "" || address.Fragment != "" || address.Path != "" {
		return "", errors.New("server must be an http(s)://host[:port] URL with no path or credentials")
	}
	if address.Scheme == "http" && !allowHTTP {
		return "", errors.New("HTTP would expose login and client credentials; use HTTPS or explicitly pass --allow-insecure-http on a trusted network")
	}
	return address.String(), nil
}

func loadIdentity(path string) (identity, error) {
	var value identity
	info, err := os.Lstat(path)
	if err != nil {
		return value, err
	}
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return value, errors.New("client identity file must be a private regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, err
	}
	if value.ID == "" || value.Credential == "" || value.Server == "" {
		return value, errors.New("incomplete client identity")
	}
	return value, nil
}

func saveIdentity(path string, value identity) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return errors.Join(writeErr, closeErr)
	}
	return nil
}

func enroll(server string) (identity, error) {
	var value identity
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return value, errors.New("first enrollment needs an interactive terminal for WebMux login")
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprint(os.Stderr, "WebMux username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		return value, err
	}
	fmt.Fprint(os.Stderr, "WebMux password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return value, err
	}
	loginBody, _ := json.Marshal(map[string]string{"username": strings.TrimSpace(username), "password": string(password)})
	var login struct {
		Token string `json:"token"`
	}
	if err := requestJSON(server+"/api/auth/login", "", loginBody, &login); err != nil {
		return value, fmt.Errorf("login: %w", err)
	}
	if login.Token == "" {
		return value, errors.New("WebMux did not return a login token")
	}
	hostname, err := os.Hostname()
	if err != nil {
		return value, err
	}
	body, _ := json.Marshal(map[string]string{"label": hostname})
	var registered struct {
		ID         string `json:"id"`
		Credential string `json:"credential"`
		PairCode   string `json:"pair_code"`
	}
	if err := requestJSON(server+"/api/clients/enroll", login.Token, body, &registered); err != nil {
		return value, fmt.Errorf("enroll: %w", err)
	}
	if registered.ID == "" || registered.Credential == "" {
		return value, errors.New("WebMux returned an incomplete client identity")
	}
	fmt.Fprintf(os.Stderr, "Client registered. To link a saved host, enter this code in its SSH tile within 15 minutes: %s\n", registered.PairCode)
	return identity{Server: server, ID: registered.ID, Credential: registered.Credential}, nil
}

func requestJSON(endpoint, bearer string, body []byte, out any) error {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2048))
	if err != nil {
		return err
	}
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("server returned %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	return json.Unmarshal(data, out)
}

func printPairCode(server, credential string) error {
	var result struct {
		PairCode string `json:"pair_code"`
	}
	if err := requestJSON(server+"/api/clients/pair-code", credential, []byte("{}"), &result); err != nil {
		return err
	}
	fmt.Println(result.PairCode)
	return nil
}

func hostSnapshot() snapshot {
	hostname, _ := os.Hostname()
	result := snapshot{Hostname: hostname, OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU()}
	var raw string
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/loadavg")
		if err == nil {
			raw = string(data)
		}
	}
	if runtime.GOOS == "darwin" {
		// sysctl is a bounded local kernel query with no shell interpolation.
		data, err := readDarwinLoad()
		if err == nil {
			raw = data
		}
	}
	for _, field := range strings.Fields(strings.Trim(raw, "{} ")) {
		if number, err := strconv.ParseFloat(field, 64); err == nil {
			result.Load1 = &number
		}
		break
	}
	return result
}

func connect(ctx context.Context, server, credential string) error {
	endpoint, err := url.Parse(server + "/api/clients/connect")
	if err != nil {
		return err
	}
	if endpoint.Scheme == "https" {
		endpoint.Scheme = "wss"
	} else {
		endpoint.Scheme = "ws"
	}
	headers := http.Header{"Authorization": {"Bearer " + credential}}
	connection, response, err := (&websocket.Dialer{HandshakeTimeout: 10 * time.Second}).DialContext(ctx, endpoint.String(), headers)
	if err != nil {
		if response != nil && response.StatusCode == 401 {
			return errRevoked
		}
		return err
	}
	defer connection.Close()
	connection.SetReadLimit(8192)
	var hello frame
	if err := connection.ReadJSON(&hello); err != nil {
		return err
	}
	if hello.Type != "hello" || hello.Protocol != protocol {
		return errors.New("unsupported WebMux client protocol")
	}
	var writeMu sync.Mutex
	write := func(id string) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = connection.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return connection.WriteJSON(frame{Type: "snapshot", ID: id, Snapshot: func() *snapshot { value := hostSnapshot(); return &value }()})
	}
	if err := write(""); err != nil {
		return err
	}
	readErrors := make(chan error, 1)
	go func() {
		for {
			var incoming frame
			if err := connection.ReadJSON(&incoming); err != nil {
				readErrors <- err
				return
			}
			if incoming.Type != "request" || incoming.Command != "system.snapshot" || incoming.ID == "" {
				readErrors <- errors.New("unsupported server request")
				return
			}
			if err := write(incoming.ID); err != nil {
				readErrors <- err
				return
			}
		}
	}()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-readErrors:
			return err
		case <-ticker.C:
			if err := write(""); err != nil {
				return err
			}
		}
	}
}

// Confirm authentication and the control protocol before telling the user a
// detached client was started. The background process makes its own connection.
func probeConnection(server, credential string) error {
	endpoint, err := url.Parse(server + "/api/clients/connect")
	if err != nil {
		return err
	}
	if endpoint.Scheme == "https" {
		endpoint.Scheme = "wss"
	} else {
		endpoint.Scheme = "ws"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	headers := http.Header{"Authorization": {"Bearer " + credential}}
	connection, response, err := (&websocket.Dialer{HandshakeTimeout: 10 * time.Second}).DialContext(ctx, endpoint.String(), headers)
	if err != nil {
		if response != nil && response.StatusCode == 401 {
			return errRevoked
		}
		return err
	}
	defer connection.Close()
	_ = connection.SetReadDeadline(time.Now().Add(10 * time.Second))
	var hello frame
	if err := connection.ReadJSON(&hello); err != nil {
		return err
	}
	if hello.Type != "hello" || hello.Protocol != protocol {
		return errors.New("unsupported WebMux client protocol")
	}
	_ = connection.SetWriteDeadline(time.Now().Add(5 * time.Second))
	facts := hostSnapshot()
	return connection.WriteJSON(frame{Type: "snapshot", Snapshot: &facts})
}
