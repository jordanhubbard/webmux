package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

const clientProtocol = 1

type clientSnapshot struct {
	Hostname string   `json:"hostname" yaml:"hostname"`
	OS       string   `json:"os" yaml:"os"`
	Arch     string   `json:"arch" yaml:"arch"`
	CPUs     int      `json:"cpus" yaml:"cpus"`
	Load1    *float64 `json:"load1,omitempty" yaml:"load1,omitempty"`
	At       string   `json:"at" yaml:"at"`
}

type clientRecord struct {
	ID         string          `json:"id" yaml:"id"`
	Owner      string          `json:"owner" yaml:"owner"`
	Label      string          `json:"label" yaml:"label"`
	SecretHash string          `json:"-" yaml:"secret_hash"`
	HostID     string          `json:"host_id,omitempty" yaml:"host_id,omitempty"`
	PairHash   string          `json:"-" yaml:"pair_hash,omitempty"`
	PairExpiry time.Time       `json:"-" yaml:"pair_expiry,omitempty"`
	LastSeen   string          `json:"last_seen,omitempty" yaml:"last_seen,omitempty"`
	Snapshot   *clientSnapshot `json:"snapshot,omitempty" yaml:"snapshot,omitempty"`
}

type clientConfig struct {
	Clients []clientRecord `yaml:"clients"`
}

type clientFrame struct {
	Type     string          `json:"type"`
	Protocol int             `json:"protocol,omitempty"`
	ID       string          `json:"id,omitempty"`
	Command  string          `json:"command,omitempty"`
	Snapshot *clientSnapshot `json:"snapshot,omitempty"`
}

type clientConnection struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[string]chan struct{}
}

func randomClientValue(n int) (string, error) {
	buffer := make([]byte, n)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func newPairCode() (string, error) {
	buffer := make([]byte, 10)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buffer), nil
}

func hashClientValue(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func (s *Server) readClients() (clientConfig, error) {
	var config clientConfig
	err := s.store.ReadConfig("clients.yaml", &config)
	return config, err
}

func (s *Server) clientIdentity(r *http.Request) (clientRecord, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return clientRecord{}, false
	}
	id, secret, ok := strings.Cut(strings.TrimPrefix(header, "Bearer "), ".")
	if !ok || len(secret) != 64 {
		return clientRecord{}, false
	}
	config, err := s.readClients()
	if err != nil {
		return clientRecord{}, false
	}
	authConfig, err := auth.LoadConfig(s.store)
	if err != nil || authConfig.Auth.Mode != "local" {
		return clientRecord{}, false
	}
	for _, client := range config.Clients {
		if client.ID == id && authConfig.HasUser(client.Owner) && subtle.ConstantTimeCompare([]byte(client.SecretHash), []byte(hashClientValue(secret))) == 1 {
			return client, true
		}
	}
	return clientRecord{}, false
}

func (s *Server) enrollClient(w http.ResponseWriter, r *http.Request, owner string) {
	authConfig, err := auth.LoadConfig(s.store)
	if err != nil || authConfig.Auth.Mode != "local" {
		writeError(w, 403, "Client enrollment requires authenticated mode")
		return
	}
	var body struct {
		Label string `json:"label"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	body.Label = strings.TrimSpace(body.Label)
	if body.Label == "" || len(body.Label) > 100 {
		writeError(w, 400, "Choose a client label of 1 to 100 characters")
		return
	}
	id, err := uuid()
	if err != nil {
		s.internalError(w, err)
		return
	}
	secret, err := randomClientValue(32)
	if err != nil {
		s.internalError(w, err)
		return
	}
	code, err := newPairCode()
	if err != nil {
		s.internalError(w, err)
		return
	}
	client := clientRecord{ID: id, Owner: owner, Label: body.Label, SecretHash: hashClientValue(secret), PairHash: hashClientValue(code), PairExpiry: time.Now().Add(15 * time.Minute)}
	if err := storage.UpdateConfig(s.store, "clients.yaml", func(config *clientConfig) error {
		config.Clients = append(config.Clients, client)
		return nil
	}); err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "credential": id + "." + secret, "pair_code": code})
}

func (s *Server) clientPairCode(w http.ResponseWriter, r *http.Request) {
	client, ok := s.clientIdentity(r)
	if !ok {
		writeError(w, 401, "Invalid client credential")
		return
	}
	code, err := newPairCode()
	if err != nil {
		s.internalError(w, err)
		return
	}
	if err := storage.UpdateConfig(s.store, "clients.yaml", func(config *clientConfig) error {
		for i := range config.Clients {
			if config.Clients[i].ID == client.ID {
				config.Clients[i].PairHash = hashClientValue(code)
				config.Clients[i].PairExpiry = time.Now().Add(15 * time.Minute)
				return nil
			}
		}
		return errors.New("client was revoked")
	}); err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"pair_code": code})
}

func (s *Server) listClients(w http.ResponseWriter, _ *http.Request, owner string) {
	config, err := s.readClients()
	if err != nil {
		s.internalError(w, err)
		return
	}
	type listed struct {
		clientRecord
		Online bool `json:"online"`
	}
	result := make([]listed, 0)
	s.clientMu.Lock()
	for _, client := range config.Clients {
		if client.Owner != owner {
			continue
		}
		_, online := s.clientSockets[client.ID]
		if fact, ok := s.clientFacts[client.ID]; ok {
			client.Snapshot = &fact
			client.LastSeen = fact.At
		}
		result = append(result, listed{client, online})
	}
	s.clientMu.Unlock()
	writeJSON(w, 200, result)
}

func (s *Server) bindClient(w http.ResponseWriter, r *http.Request, owner string) {
	var body struct {
		Code string `json:"pair_code"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	code := strings.ToUpper(strings.TrimSpace(body.Code))
	if len(code) != 16 {
		writeError(w, 400, "Invalid pairing code")
		return
	}
	hostID := r.PathValue("id")
	var hosts hostsConfig
	if err := s.store.ReadConfig("hosts.yaml", &hosts); err != nil {
		s.internalError(w, err)
		return
	}
	found := false
	for _, host := range hosts.Hosts {
		if host["id"] == hostID {
			found = true
			break
		}
	}
	if !found {
		writeError(w, 404, "Host not found")
		return
	}
	var bound string
	err := storage.UpdateConfig(s.store, "clients.yaml", func(config *clientConfig) error {
		for _, client := range config.Clients {
			if client.Owner == owner && client.HostID == hostID {
				return &responseError{409, "Host already has a linked client"}
			}
		}
		for i := range config.Clients {
			client := &config.Clients[i]
			if client.Owner == owner && client.PairHash != "" && time.Now().Before(client.PairExpiry) && subtle.ConstantTimeCompare([]byte(client.PairHash), []byte(hashClientValue(code))) == 1 {
				if client.HostID != "" {
					return &responseError{409, "Unlink this client from its current host first"}
				}
				client.HostID = hostID
				client.PairHash = ""
				client.PairExpiry = time.Time{}
				bound = client.ID
				return nil
			}
		}
		return &responseError{404, "Pairing code not found or expired"}
	})
	if err != nil {
		s.catalogError(w, err, "Failed to bind client")
		return
	}
	writeJSON(w, 200, map[string]string{"client_id": bound, "host_id": hostID})
}

func (s *Server) unbindClient(w http.ResponseWriter, r *http.Request, owner string) {
	hostID := r.PathValue("id")
	err := storage.UpdateConfig(s.store, "clients.yaml", func(config *clientConfig) error {
		for i := range config.Clients {
			if config.Clients[i].Owner == owner && config.Clients[i].HostID == hostID {
				config.Clients[i].HostID = ""
				return nil
			}
		}
		return &responseError{404, "Linked client not found"}
	})
	if err != nil {
		s.catalogError(w, err, "Failed to unlink client")
		return
	}
	w.WriteHeader(204)
}

func (s *Server) revokeClient(w http.ResponseWriter, r *http.Request, owner string) {
	id := r.PathValue("id")
	err := storage.UpdateConfig(s.store, "clients.yaml", func(config *clientConfig) error {
		for i, client := range config.Clients {
			if client.ID == id && client.Owner == owner {
				config.Clients = append(config.Clients[:i], config.Clients[i+1:]...)
				return nil
			}
		}
		return &responseError{404, "Client not found"}
	})
	if err != nil {
		s.catalogError(w, err, "Failed to revoke client")
		return
	}
	s.clientMu.Lock()
	if current := s.clientSockets[id]; current != nil {
		_ = current.conn.Close()
	}
	delete(s.clientFacts, id)
	s.clientMu.Unlock()
	w.WriteHeader(204)
}

func (c *clientConnection) write(frame clientFrame) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return c.conn.WriteJSON(frame)
}

func (s *Server) clientSocket(w http.ResponseWriter, r *http.Request) {
	client, ok := s.clientIdentity(r)
	if !ok {
		writeError(w, 401, "Invalid client credential")
		return
	}
	s.socketMu.Lock()
	if s.closing {
		s.socketMu.Unlock()
		writeError(w, 503, "Server is shutting down")
		return
	}
	s.socketWorkers.Add(1)
	s.socketMu.Unlock()
	defer s.socketWorkers.Done()
	connection, err := (&websocket.Upgrader{HandshakeTimeout: 5 * time.Second}).Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer connection.Close()
	connection.SetReadLimit(8192)
	state := &clientConnection{conn: connection, pending: make(map[string]chan struct{})}
	s.clientMu.Lock()
	if previous := s.clientSockets[client.ID]; previous != nil {
		_ = previous.conn.Close()
	}
	s.clientSockets[client.ID] = state
	s.clientMu.Unlock()
	defer func() {
		s.clientMu.Lock()
		current := s.clientSockets[client.ID] == state
		if current {
			delete(s.clientSockets, client.ID)
		}
		fact, hasFact := s.clientFacts[client.ID]
		s.clientMu.Unlock()
		if current && hasFact {
			_ = storage.UpdateConfig(s.store, "clients.yaml", func(config *clientConfig) error {
				for i := range config.Clients {
					if config.Clients[i].ID == client.ID {
						config.Clients[i].Snapshot = &fact
						config.Clients[i].LastSeen = fact.At
					}
				}
				return nil
			})
		}
	}()
	if err := state.write(clientFrame{Type: "hello", Protocol: clientProtocol, ID: client.ID}); err != nil {
		return
	}
	for {
		_ = connection.SetReadDeadline(time.Now().Add(50 * time.Second))
		var frame clientFrame
		if err := connection.ReadJSON(&frame); err != nil {
			return
		}
		if _, valid := s.clientIdentity(r); !valid {
			closeSocket(connection, 1008, "Client credential revoked")
			return
		}
		if frame.Type != "snapshot" || frame.Snapshot == nil || len(frame.Snapshot.Hostname) > 255 || len(frame.Snapshot.OS) == 0 || len(frame.Snapshot.OS) > 32 || len(frame.Snapshot.Arch) == 0 || len(frame.Snapshot.Arch) > 32 || frame.Snapshot.CPUs < 1 || frame.Snapshot.CPUs > 65536 || (frame.Snapshot.Load1 != nil && (math.IsNaN(*frame.Snapshot.Load1) || math.IsInf(*frame.Snapshot.Load1, 0) || *frame.Snapshot.Load1 < 0)) {
			closeSocket(connection, 1008, "Invalid client snapshot")
			return
		}
		frame.Snapshot.At = time.Now().UTC().Format(time.RFC3339)
		s.clientMu.Lock()
		if s.clientSockets[client.ID] == state {
			s.clientFacts[client.ID] = *frame.Snapshot
		}
		s.clientMu.Unlock()
		if frame.ID != "" {
			state.mu.Lock()
			pending := state.pending[frame.ID]
			delete(state.pending, frame.ID)
			state.mu.Unlock()
			if pending != nil {
				close(pending)
			}
		}
	}
}

func (s *Server) refreshClient(w http.ResponseWriter, r *http.Request, owner string) {
	id := r.PathValue("id")
	config, err := s.readClients()
	if err != nil {
		s.internalError(w, err)
		return
	}
	authorized := false
	for _, client := range config.Clients {
		if client.ID == id && client.Owner == owner {
			authorized = true
			break
		}
	}
	if !authorized {
		writeError(w, 404, "Client not found")
		return
	}
	s.clientMu.Lock()
	connection := s.clientSockets[id]
	s.clientMu.Unlock()
	if connection == nil {
		writeError(w, 409, "Client is offline")
		return
	}
	requestID, err := randomClientValue(12)
	if err != nil {
		s.internalError(w, err)
		return
	}
	done := make(chan struct{})
	connection.mu.Lock()
	connection.pending[requestID] = done
	connection.mu.Unlock()
	defer func() {
		connection.mu.Lock()
		delete(connection.pending, requestID)
		connection.mu.Unlock()
	}()
	if err := connection.write(clientFrame{Type: "request", ID: requestID, Command: "system.snapshot"}); err != nil {
		writeError(w, 502, "Client connection failed")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	select {
	case <-done:
		s.clientMu.Lock()
		fact := s.clientFacts[id]
		s.clientMu.Unlock()
		writeJSON(w, 200, fact)
	case <-ctx.Done():
		writeError(w, 504, "Client did not respond")
	}
}
