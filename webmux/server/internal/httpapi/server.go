// Package httpapi implements the browser-facing HTTP contract.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/jordanhubbard/webmux/server/internal/ai"
	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/desktop"
	"github.com/jordanhubbard/webmux/server/internal/netguard"
	"github.com/jordanhubbard/webmux/server/internal/session"
	"github.com/jordanhubbard/webmux/server/internal/storage"
	"github.com/jordanhubbard/webmux/server/internal/upload"
)

type Server struct {
	store         *storage.Store
	auth          *auth.Service
	name          string
	secure        bool
	webDir        string
	webFS         fs.FS
	passwordSlots chan struct{}
	logger        *slog.Logger
	sessions      *session.Broker
	vnc           *desktop.Broker
	rdp           *desktop.Broker
	targets       *netguard.Guard
	uploads       *upload.Service
	ai            *ai.Service
	socketMu      sync.Mutex
	sockets       map[*websocket.Conn]struct{}
	socketWorkers sync.WaitGroup
	closing       bool
	closeOnce     sync.Once
	closeErr      error
}

type Options struct {
	Name       string
	SecureMode bool
	JWTSecret  string
	Logger     *slog.Logger
	WebDir     string
	WebFS      fs.FS
}

func New(store *storage.Store, options Options) (*Server, error) {
	service, err := auth.New(store, options.JWTSecret)
	if err != nil {
		return nil, err
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	sessions, err := session.New(store, logger)
	if err != nil {
		return nil, err
	}
	vnc, err := desktop.New(store, desktop.VNC)
	if err != nil {
		return nil, err
	}
	rdp, err := desktop.New(store, desktop.RDP)
	if err != nil {
		return nil, err
	}
	return &Server{store: store, auth: service, name: options.Name, secure: options.SecureMode, webDir: options.WebDir, webFS: options.WebFS, passwordSlots: make(chan struct{}, 2), logger: logger, sessions: sessions, vnc: vnc, rdp: rdp, targets: netguard.New(os.Getenv("WEBMUX_ALLOW_LOCAL_TARGETS") == "1"), uploads: upload.New(store), ai: ai.New(nil, nil)}, nil
}

func (s *Server) RestoreSessions() error {
	return errors.Join(s.sessions.Restore(), s.vnc.Restore(), s.rdp.Restore())
}
func (s *Server) StartSlave(host string, port int) error {
	_, err := s.sessions.ResetForSlave(host, port)
	return err
}
func (s *Server) Close() error { return s.closeSocketsAndSessions() }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok", "name": s.name})
	})
	mux.HandleFunc("GET /api/auth/status", s.authStatus)
	limited := newLimiter(10, authWindow)
	mux.Handle("POST /api/auth/login", limited.wrap(http.HandlerFunc(s.login)))
	mux.Handle("POST /api/auth/bootstrap", limited.wrap(http.HandlerFunc(s.bootstrap)))
	mux.Handle("POST /api/auth/register", limited.wrap(s.protected(true, s.register)))
	mux.Handle("POST /api/auth/refresh", s.protected(false, s.refresh))
	mux.Handle("GET /api/auth/me", s.protected(false, s.me))
	mux.Handle("GET /api/auth/users", s.protected(true, s.users))
	mux.Handle("DELETE /api/auth/users/{username}", s.protected(true, s.deleteUser))
	mux.Handle("POST /api/auth/ticket", s.protected(false, s.issueTicket))
	mux.Handle("GET /api/hosts", s.protected(false, s.listHosts))
	mux.Handle("POST /api/hosts", s.protected(false, s.createHost))
	mux.Handle("PUT /api/hosts/{id}", s.protected(false, s.updateHost))
	mux.Handle("DELETE /api/hosts/{id}", s.protected(false, s.deleteHost))
	mux.Handle("GET /api/keys", s.protected(false, s.listKeys))
	mux.Handle("POST /api/keys", s.protected(false, s.createKey))
	mux.Handle("POST /api/upload", s.protected(false, s.uploadFile))
	mux.Handle("GET /api/ai/status", s.protected(false, s.aiStatus))
	mux.Handle("POST /api/ai/chat", s.protected(false, s.aiChat))
	mux.Handle("DELETE /api/keys/{id}", s.protected(false, s.deleteKey))
	mux.Handle("GET /api/config", s.protected(false, s.getSettings))
	mux.Handle("PUT /api/config", s.protected(false, s.updateSettings))
	mux.Handle("GET /api/config/layout", s.protected(false, s.getLayout))
	mux.Handle("PUT /api/config/layout", s.protected(false, s.updateLayout))
	mux.Handle("GET /api/config/fonts/{index}", s.protected(false, s.getFont))
	mux.Handle("GET /api/sessions", s.protected(false, s.listSessions))
	mux.Handle("GET /api/sessions/templates", s.protected(false, s.listTemplates))
	mux.Handle("GET /api/sessions/templates/{id}", s.protected(false, s.getTemplate))
	mux.Handle("POST /api/sessions", s.protected(false, s.createSession))
	mux.Handle("GET /api/sessions/{id}", s.protected(false, s.getSession))
	mux.Handle("PATCH /api/sessions/{id}", s.protected(false, s.patchSession))
	mux.Handle("DELETE /api/sessions/{id}", s.protected(false, s.deleteSession))
	mux.Handle("POST /api/sessions/{id}/reconnect", s.protected(false, s.reconnectSession))
	mux.HandleFunc("GET /api/term/{id}", s.terminalSocket)
	mux.Handle("GET /api/agents/config", s.protected(false, s.getAgentConfig))
	mux.Handle("GET /api/agents/sessions", s.protected(false, s.listAgents))
	mux.Handle("GET /api/agents/{agentId}/sessions", s.protected(false, s.listAgents))
	mux.Handle("POST /api/agents/{agentId}/attach", s.protected(false, s.attachAgent))
	mux.Handle("POST /api/agents/{agentId}/scratch", s.protected(false, s.scratchAgent))
	s.registerDesktops(mux)
	mux.HandleFunc("GET /api/vnc/ws/{id}", s.vncSocket)
	mux.HandleFunc("GET /api/rdp/ws/{id}", s.rdpSocket)
	if s.webDir != "" || s.webFS != nil {
		mux.HandleFunc("GET /", s.serveUI)
	}
	return s.cors(newLimiter(300, globalWindow).wrap(apiPaths(mux)))
}

// Express accepts trailing slashes on API routes without redirecting. Preserve
// that behavior, but don't strip a slash encoded inside an identifier (%2F).
func apiPaths(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && strings.HasSuffix(r.URL.EscapedPath(), "/") {
			clone := r.Clone(r.Context())
			clone.URL.Path = strings.TrimRight(clone.URL.Path, "/")
			clone.URL.RawPath = strings.TrimRight(clone.URL.RawPath, "/")
			r = clone
		}
		next.ServeHTTP(w, r)
	})
}

type authenticatedHandler func(http.ResponseWriter, *http.Request, string)

func (s *Server) protected(admin bool, next authenticatedHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		config, err := auth.LoadConfig(s.store)
		if err != nil {
			s.internalError(w, err)
			return
		}
		if config.Auth.Mode == "none" {
			if admin {
				writeError(w, 401, "Unauthorized")
				return
			}
			next(w, r, "anonymous")
			return
		}
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") || len(header) == len("Bearer ") {
			writeError(w, 401, "Unauthorized")
			return
		}
		username, err := s.auth.Verify(strings.TrimPrefix(header, "Bearer "))
		if err != nil || !config.HasUser(username) {
			writeError(w, 401, "Invalid or expired token")
			return
		}
		if admin && !auth.IsAdmin(config.Auth.Users, username) {
			writeError(w, 403, "Admin privileges required")
			return
		}
		next(w, r, username)
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.secure {
			w.Header().Add("Vary", "Origin")
			if origin := r.Header.Get("Origin"); origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET,HEAD,PUT,PATCH,POST,DELETE")
				w.Header().Add("Vary", "Access-Control-Request-Headers")
				if headers := r.Header.Get("Access-Control-Request-Headers"); headers != "" {
					w.Header().Set("Access-Control-Allow-Headers", headers)
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(out); err != nil {
		jsonError(w, err)
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		jsonError(w, err)
		return false
	}
	return true
}

func jsonError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, 413, "Request body too large")
		return
	}
	writeError(w, 400, "Invalid JSON request body")
}

func writeJSON(w http.ResponseWriter, code int, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		code = http.StatusInternalServerError
		encoded = []byte(`{"error":"Internal server error"}`)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write(append(encoded, '\n'))
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{"error": message})
}

func (s *Server) internalError(w http.ResponseWriter, err error) {
	s.logger.Error("request failed", "error", err)
	writeError(w, 500, "Internal server error")
}

func (s *Server) event(fields map[string]any) {
	if err := s.store.AppendEvent(fields); err != nil {
		s.logger.Error("audit event failed", "error", err)
	}
}
