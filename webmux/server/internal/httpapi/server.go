// Package httpapi implements the browser-facing HTTP contract.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

type Server struct {
	store         *storage.Store
	auth          *auth.Service
	name          string
	secure        bool
	passwordSlots chan struct{}
	logger        *slog.Logger
}

type Options struct {
	Name       string
	SecureMode bool
	JWTSecret  string
	Logger     *slog.Logger
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
	return &Server{store: store, auth: service, name: options.Name, secure: options.SecureMode, passwordSlots: make(chan struct{}, 2), logger: logger}, nil
}

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
	mux.Handle("DELETE /api/keys/{id}", s.protected(false, s.deleteKey))
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
