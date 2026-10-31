package httpapi

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/gorilla/websocket"
	"github.com/jordanhubbard/webmux/server/internal/auth"
)

// Federation is file-managed: neither URLs nor secrets may be supplied by a
// browser. Each inbound credential selects exactly one local session owner.
type serverPeer struct {
	ID        string `yaml:"id" json:"id"`
	Name      string `yaml:"name" json:"name"`
	URL       string `yaml:"url" json:"-"`
	TokenEnv  string `yaml:"token_env" json:"-"`
	AllowHTTP bool   `yaml:"allow_http" json:"-"`
}
type serverGrant struct {
	ID       string `yaml:"id"`
	Owner    string `yaml:"owner"`
	TokenEnv string `yaml:"token_env"`
}
type serverDirectory struct {
	Version int           `yaml:"version"`
	Servers []serverPeer  `yaml:"servers"`
	Grants  []serverGrant `yaml:"grants"`
}

var federationID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func (s *Server) serverDirectory() (serverDirectory, error) {
	var d serverDirectory
	err := s.store.ReadConfig("servers.yaml", &d)
	if errors.Is(err, os.ErrNotExist) {
		return d, nil
	}
	if err != nil {
		return d, err
	}
	if d.Version != 1 {
		return d, errors.New("servers.yaml requires version: 1")
	}
	if len(d.Servers) > 32 || len(d.Grants) > 32 {
		return d, errors.New("server directory permits at most 32 peers and 32 grants")
	}
	seen := map[string]bool{"local": true, "all": true}
	for _, p := range d.Servers {
		u, err := url.Parse(p.URL)
		if !federationID.MatchString(p.ID) || seen[p.ID] || p.Name == "" || p.TokenEnv == "" || err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && !(u.Scheme == "http" && p.AllowHTTP)) {
			return d, errors.New("invalid server directory entry (unique ID, name, origin URL and token_env required; HTTP requires allow_http)")
		}
		seen[p.ID] = true
	}
	seen = map[string]bool{}
	tokens := map[string]bool{}
	for _, g := range d.Grants {
		token := os.Getenv(g.TokenEnv)
		if !federationID.MatchString(g.ID) || seen[g.ID] || g.Owner == "" || g.TokenEnv == "" || (token != "" && tokens[token]) {
			return d, errors.New("invalid or duplicate server grant")
		}
		seen[g.ID] = true
		if token != "" {
			tokens[token] = true
		}
	}
	return d, nil
}
func (s *Server) federationAccess(w http.ResponseWriter, owner string) bool {
	c, err := auth.LoadConfig(s.store)
	if err != nil {
		s.internalError(w, err)
		return false
	}
	if c.Auth.Mode != "none" && !auth.IsAdmin(c.Auth.Users, owner) {
		writeError(w, 403, "Administrator access is required for connected servers")
		return false
	}
	return true
}
func (s *Server) federationOwnerAllowed(owner string) bool {
	c, err := auth.LoadConfig(s.store)
	return err == nil && ((c.Auth.Mode == "none" && owner == "anonymous") || (c.Auth.Mode == "local" && c.HasUser(owner)))
}
func (s *Server) federationGrant(r *http.Request) (serverGrant, bool) {
	d, err := s.serverDirectory()
	if err != nil {
		return serverGrant{}, false
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == r.Header.Get("Authorization") || len(token) < 32 {
		return serverGrant{}, false
	}
	for _, g := range d.Grants {
		expected := os.Getenv(g.TokenEnv)
		if len(expected) >= 32 && subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1 && s.federationOwnerAllowed(g.Owner) {
			return g, true
		}
	}
	return serverGrant{}, false
}

type federationContextKey struct{}

func (s *Server) federationInbound(next authenticatedHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g, ok := s.federationGrant(r)
		if !ok {
			writeError(w, 401, "Invalid server credential or owner grant")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), federationContextKey{}, g)), g.Owner)
	})
}
func (s *Server) listServers(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.federationAccess(w, owner) {
		return
	}
	d, err := s.serverDirectory()
	if err != nil {
		s.internalError(w, err)
		return
	}
	peers := []serverPeer{{ID: "local", Name: s.name}}
	peers = append(peers, d.Servers...)
	writeJSON(w, 200, map[string]any{"version": 1, "servers": peers})
}
func (s *Server) peer(id string) (serverPeer, string, error) {
	d, err := s.serverDirectory()
	if err != nil {
		return serverPeer{}, "", err
	}
	for _, p := range d.Servers {
		if p.ID == id {
			token := os.Getenv(p.TokenEnv)
			if len(token) < 32 {
				return p, "", errors.New("server credential is unavailable (at least 32 characters required)")
			}
			return p, token, nil
		}
	}
	return serverPeer{}, "", errors.New("server is not configured")
}

// The proxy exposes only this fixed session contract, never general config,
// account, coordinator or arbitrary upstream paths. Redirects are not followed.
func (s *Server) proxyServer(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.federationAccess(w, owner) {
		return
	}
	id, action := r.PathValue("id"), r.PathValue("action")
	if action != "" && action != "messages" && action != "reconnect" {
		writeError(w, 404, "Unknown server action")
		return
	}
	if r.PathValue("server") == "local" {
		s.localFederationAction(w, r, owner)
		return
	}
	peer, token, err := s.peer(r.PathValue("server"))
	if err != nil {
		writeError(w, 503, err.Error())
		return
	}
	target, _ := url.Parse(peer.URL)
	path := "/api/federation/sessions"
	if id != "" {
		if !federationID.MatchString(id) {
			writeError(w, 400, "Invalid session ID")
			return
		}
		path += "/" + id
	}
	if action != "" {
		path += "/" + action
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.Clone(ctx)
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	proxy := &httputil.ReverseProxy{
		Rewrite: func(p *httputil.ProxyRequest) {
			p.SetURL(target)
			p.Out.URL.Path = path
			p.Out.URL.RawPath = ""
			p.Out.URL.RawQuery = ""
			p.Out.Header = http.Header{"Authorization": {"Bearer " + token}, "Content-Type": {"application/json"}}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, 502, "Connected server is unavailable")
		},
		ModifyResponse: func(r *http.Response) error {
			// Do not expose upstream cookies, authentication challenges or redirects.
			if r.StatusCode >= 300 && r.StatusCode < 400 {
				return errors.New("server redirect refused")
			}
			if r.StatusCode == 401 || r.StatusCode == 403 {
				r.StatusCode = 502
			}
			r.Header = http.Header{"Content-Type": {"application/json"}}
			return nil
		},
	}
	proxy.ServeHTTP(w, r)
}
func (s *Server) localFederationAction(w http.ResponseWriter, r *http.Request, owner string) {
	switch {
	case r.PathValue("action") == "messages":
		s.federationMessage(w, r, owner)
	case r.PathValue("action") == "reconnect":
		s.reconnectSession(w, r, owner)
	case r.PathValue("id") == "" && r.Method == "GET":
		s.listSessions(w, r, owner)
	case r.PathValue("id") == "" && r.Method == "POST":
		s.createSession(w, r, owner)
	default:
		writeError(w, 404, "Unknown server action")
	}
}
func (s *Server) federationMessage(w http.ResponseWriter, r *http.Request, owner string) {
	var body struct {
		Message string `json:"message"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if len(strings.TrimSpace(body.Message)) == 0 || len(body.Message) > 16*1024 || strings.ContainsFunc(body.Message, unicode.IsControl) {
		writeError(w, 400, "Message must be one nonempty line, at most 16 KiB, without terminal control sequences")
		return
	}
	current, err := s.sessions.Get(owner, r.PathValue("id"))
	if err != nil {
		s.sessionError(w, err, "Cannot send message")
		return
	}
	if current.State != "connected" {
		writeError(w, 409, "Session is not connected; message was not queued")
		return
	}
	source := "local:" + owner
	if grant, ok := r.Context().Value(federationContextKey{}).(serverGrant); ok {
		source = grant.ID
	}
	// Bracketed paste keeps embedded punctuation in the input buffer; Enter is
	// explicit. This is terminal input admission, never a processing receipt.
	data := "\x1b[200~[WebMux " + source + "] " + body.Message + "\x1b[201~\r"
	if err := s.sessions.InputIfConnected(owner, current.ID, data); err != nil {
		writeError(w, 409, "Session input is unavailable; message was not queued")
		return
	}
	s.event(map[string]any{"type": "server_message_queued", "source": source, "owner": owner, "session_id": current.ID})
	writeJSON(w, 202, map[string]any{"queued": true, "processed": false, "session_id": current.ID, "source": source})
}
func (s *Server) registerFederation(mux *http.ServeMux) {
	mux.Handle("GET /api/servers", s.protected(false, s.listServers))
	for _, pattern := range []string{"GET /api/servers/{server}/sessions", "POST /api/servers/{server}/sessions", "POST /api/servers/{server}/sessions/{id}/{action}"} {
		mux.Handle(pattern, s.protected(false, s.proxyServer))
	}
	mux.HandleFunc("GET /api/servers/{server}/term/{id}", s.serverTerminal)
	for _, pattern := range []string{"GET /api/federation/sessions", "POST /api/federation/sessions", "POST /api/federation/sessions/{id}/{action}"} {
		mux.Handle(pattern, s.federationInbound(s.localFederationAction))
	}
	mux.Handle("GET /api/federation/term/{id}", s.federationInbound(func(w http.ResponseWriter, r *http.Request, _ string) { s.terminalSocket(w, r) }))
}

func (s *Server) serverTerminal(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("server") == "local" {
		s.terminalSocket(w, r)
		return
	}
	owner, mode, ok := s.socketIdentity(r)
	if !ok {
		writeError(w, 401, "Unauthorized")
		return
	}
	if !s.federationAccess(w, owner) {
		return
	}
	id := r.PathValue("id")
	if !federationID.MatchString(id) {
		writeError(w, 400, "Invalid session ID")
		return
	}
	peer, token, err := s.peer(r.PathValue("server"))
	if err != nil {
		writeError(w, 503, err.Error())
		return
	}
	target, _ := url.Parse(peer.URL)
	if target.Scheme == "https" {
		target.Scheme = "wss"
	} else {
		target.Scheme = "ws"
	}
	target.Path = "/api/federation/term/" + id
	upstream, _, err := (&websocket.Dialer{HandshakeTimeout: 5 * time.Second}).DialContext(r.Context(), target.String(), http.Header{"Authorization": {"Bearer " + token}})
	if err != nil {
		writeError(w, 502, "Connected server terminal is unavailable")
		return
	}
	defer upstream.Close()
	s.socketMu.Lock()
	if s.closing {
		s.socketMu.Unlock()
		writeError(w, 503, "Server is shutting down")
		return
	}
	s.socketWorkers.Add(1)
	s.socketMu.Unlock()
	defer s.socketWorkers.Done()
	downstream, err := (&websocket.Upgrader{HandshakeTimeout: 5 * time.Second}).Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer downstream.Close()
	s.socketMu.Lock()
	if s.closing {
		s.socketMu.Unlock()
		return
	}
	if s.sockets == nil {
		s.sockets = map[*websocket.Conn]struct{}{}
	}
	s.sockets[downstream] = struct{}{}
	s.socketMu.Unlock()
	defer func() { s.socketMu.Lock(); delete(s.sockets, downstream); s.socketMu.Unlock() }()
	done := make(chan error, 2)
	bridge := func(dst, src *websocket.Conn) {
		for {
			kind, data, err := src.ReadMessage()
			if err != nil {
				done <- err
				return
			}
			if err = dst.SetWriteDeadline(time.Now().Add(10 * time.Second)); err == nil {
				err = dst.WriteMessage(kind, data)
			}
			if err != nil {
				done <- err
				return
			}
		}
	}
	downstream.SetReadLimit(1 << 20)
	upstream.SetReadLimit(1 << 20)
	_ = upstream.SetReadDeadline(time.Now().Add(15 * time.Second))
	upstream.SetPongHandler(func(string) error { return upstream.SetReadDeadline(time.Now().Add(15 * time.Second)) })
	go bridge(upstream, downstream)
	go bridge(downstream, upstream)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			code, reason := 1001, "Server connection closed"
			var closed *websocket.CloseError
			if errors.As(err, &closed) && (closed.Code == 1000 || closed.Code == 1008) {
				code, reason = closed.Code, closed.Text
			}
			closeSocket(downstream, code, reason)
			_ = upstream.Close()
			<-done
			return
		case <-ticker.C:
			if err := upstream.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)); err != nil {
				closeSocket(downstream, 1001, "Connected server is unavailable")
				_ = upstream.Close()
				<-done
				<-done
				return
			}
			currentPeer, newToken, err := s.peer(peer.ID)
			if err != nil || newToken != token || currentPeer != peer || !s.socketAuthorized(owner, mode) || !s.federationAdmin(owner) {
				closeSocket(downstream, 1008, "Server access revoked")
				_ = upstream.Close()
				<-done
				<-done
				return
			}
		}
	}
}
func (s *Server) federationAdmin(owner string) bool {
	c, err := auth.LoadConfig(s.store)
	return err == nil && (c.Auth.Mode == "none" || auth.IsAdmin(c.Auth.Users, owner))
}
