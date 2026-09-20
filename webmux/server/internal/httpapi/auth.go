package httpapi

import (
	"errors"
	"net/http"
	"unicode/utf16"

	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Admin    bool   `json:"admin"`
}

type account struct {
	Username string `json:"username"`
	Admin    bool   `json:"admin"`
}

type responseError struct {
	status  int
	message string
}

func (e *responseError) Error() string { return e.message }

func (s *Server) changeError(w http.ResponseWriter, err error) {
	var response *responseError
	if errors.As(err, &response) {
		writeError(w, response.status, response.message)
	} else {
		s.internalError(w, err)
	}
}

func readCredentials(w http.ResponseWriter, r *http.Request) (credentials, bool) {
	var body credentials
	if !decodeJSON(w, r, &body) {
		return body, false
	}
	if body.Username == "" || body.Password == "" {
		writeError(w, 400, "Username and password required")
		return body, false
	}
	return body, true
}

// Keep expensive password work bounded across all clients, independently of
// per-IP rate limits. Waiting requests release their slot on cancellation.
func (s *Server) passwordSlot(r *http.Request) bool {
	select {
	case s.passwordSlots <- struct{}{}:
		return true
	case <-r.Context().Done():
		return false
	}
}

func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	config, err := auth.LoadConfig(s.store)
	if err != nil {
		writeJSON(w, 200, map[string]any{"mode": "local", "bootstrap_required": true})
		return
	}
	writeJSON(w, 200, map[string]any{"mode": config.Auth.Mode, "bootstrap_required": len(config.Auth.Users) == 0})
}

func (s *Server) token(w http.ResponseWriter, username, mode string) {
	token, err := s.auth.Sign(username)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"token": token, "mode": mode})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	body, ok := readCredentials(w, r)
	if !ok {
		return
	}
	config, err := auth.LoadConfig(s.store)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if config.Auth.Mode == "none" {
		s.token(w, body.Username, "none")
		return
	}
	for _, user := range config.Auth.Users {
		if user.Username != body.Username {
			continue
		}
		if !s.passwordSlot(r) {
			return
		}
		valid, err := auth.VerifyPassword(user.PasswordHash, body.Password)
		<-s.passwordSlots
		if err != nil {
			s.internalError(w, err)
			return
		}
		if valid {
			s.event(map[string]any{"type": "login_success", "username": body.Username})
			s.token(w, body.Username, "local")
			return
		}
		break
	}
	s.event(map[string]any{"type": "login_failed", "username": body.Username})
	writeError(w, 401, "Invalid credentials")
}

func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	body, ok := readCredentials(w, r)
	if !ok {
		return
	}
	config, err := auth.LoadConfig(s.store)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if len(config.Auth.Users) > 0 {
		writeError(w, 403, "Bootstrap not available — accounts already exist")
		return
	}
	if !s.passwordSlot(r) {
		return
	}
	hash, err := auth.HashPassword(body.Password)
	<-s.passwordSlots
	if err != nil {
		s.internalError(w, err)
		return
	}
	err = storage.UpdateConfig(s.store, "auth.yaml", func(c *auth.Config) error {
		if err := c.Validate(); err != nil {
			return err
		}
		if len(c.Auth.Users) > 0 {
			return &responseError{403, "Bootstrap not available — accounts already exist"}
		}
		c.Auth.Users = []auth.User{{Username: body.Username, PasswordHash: hash, Admin: true}}
		return nil
	})
	if err != nil {
		s.changeError(w, err)
		return
	}
	s.event(map[string]any{"type": "bootstrap_complete", "username": body.Username})
	s.token(w, body.Username, "local")
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request, username string) {
	mode := "local"
	if username == "anonymous" {
		mode = "none"
	}
	s.token(w, username, mode)
}

func checkAdmin(config *auth.Config, actor string) error {
	if err := config.Validate(); err != nil {
		return err
	}
	if config.Auth.Mode == "none" {
		return &responseError{403, "User management is unavailable when auth is disabled"}
	}
	if !config.HasUser(actor) || !auth.IsAdmin(config.Auth.Users, actor) {
		return &responseError{403, "Admin privileges required"}
	}
	return nil
}

func (s *Server) register(w http.ResponseWriter, r *http.Request, actor string) {
	body, ok := readCredentials(w, r)
	if !ok {
		return
	}
	// JavaScript string length counts UTF-16 code units, not UTF-8 bytes.
	length := len(utf16.Encode([]rune(body.Username)))
	if length < 2 || length > 64 {
		writeError(w, 400, "Username must be 2-64 characters")
		return
	}
	if len(utf16.Encode([]rune(body.Password))) < 4 {
		writeError(w, 400, "Password must be at least 4 characters")
		return
	}
	if !s.passwordSlot(r) {
		return
	}
	hash, err := auth.HashPassword(body.Password)
	<-s.passwordSlots
	if err != nil {
		s.internalError(w, err)
		return
	}
	err = storage.UpdateConfig(s.store, "auth.yaml", func(c *auth.Config) error {
		if err := checkAdmin(c, actor); err != nil {
			return err
		}
		if c.HasUser(body.Username) {
			return &responseError{409, "Username already exists"}
		}
		c.Auth.Users = append(c.Auth.Users, auth.User{Username: body.Username, PasswordHash: hash, Admin: body.Admin})
		return nil
	})
	if err != nil {
		s.changeError(w, err)
		return
	}
	s.event(map[string]any{"type": "account_created", "username": body.Username, "admin": body.Admin, "created_by": actor})
	writeJSON(w, 201, account{body.Username, body.Admin})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, username string) {
	config, err := auth.LoadConfig(s.store)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, 200, account{username, config.Auth.Mode != "none" && auth.IsAdmin(config.Auth.Users, username)})
}

func (s *Server) users(w http.ResponseWriter, r *http.Request, actor string) {
	config, err := auth.LoadConfig(s.store)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if err := checkAdmin(&config, actor); err != nil {
		s.changeError(w, err)
		return
	}
	users := make([]account, 0, len(config.Auth.Users))
	for _, user := range config.Auth.Users {
		users = append(users, account{user.Username, auth.IsAdmin(config.Auth.Users, user.Username)})
	}
	writeJSON(w, 200, users)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request, actor string) {
	target := r.PathValue("username")
	if target == actor {
		writeError(w, 400, "You cannot remove your own account")
		return
	}
	err := storage.UpdateConfig(s.store, "auth.yaml", func(c *auth.Config) error {
		if err := checkAdmin(c, actor); err != nil {
			return err
		}
		for i, user := range c.Auth.Users {
			if user.Username == target {
				c.Auth.Users = append(c.Auth.Users[:i], c.Auth.Users[i+1:]...)
				return nil
			}
		}
		return &responseError{404, "User not found"}
	})
	if err != nil {
		s.changeError(w, err)
		return
	}
	s.event(map[string]any{"type": "account_deleted", "username": target, "deleted_by": actor})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) issueTicket(w http.ResponseWriter, r *http.Request, username string) {
	ticket, err := s.auth.IssueTicket(username)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ticket": ticket, "expires_in": int(auth.TicketTTL.Seconds())})
}
