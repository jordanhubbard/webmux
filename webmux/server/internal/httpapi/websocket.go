package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/jordanhubbard/webmux/server/internal/agent"
	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/session"
)

func (s *Server) closeSocketsAndSessions() error {
	s.closeOnce.Do(func() {
		s.socketMu.Lock()
		s.closing = true
		for connection := range s.sockets {
			_ = connection.Close()
		}
		s.socketMu.Unlock()
		s.closeErr = errors.Join(s.sessions.Close(), s.vnc.Close(), s.rdp.Close(), s.uploads.Close())
		s.socketWorkers.Wait()
	})
	return s.closeErr
}

func (s *Server) socketIdentity(r *http.Request) (owner, mode string, ok bool) {
	config, err := auth.LoadConfig(s.store)
	if err != nil {
		return "", "", false
	}
	mode = config.Auth.Mode
	if ticket := r.URL.Query().Get("ticket"); ticket != "" {
		owner, ok = s.auth.ConsumeTicket(ticket)
		if !ok {
			return "", mode, false
		}
	} else if mode == "local" {
		owner, err = s.auth.Verify(r.URL.Query().Get("token"))
		if err != nil {
			return "", mode, false
		}
	}
	if mode == "none" {
		return "anonymous", mode, true
	}
	return owner, mode, config.HasUser(owner)
}
func (s *Server) socketAuthorized(owner, mode string) bool {
	config, err := auth.LoadConfig(s.store)
	return err == nil && config.Auth.Mode == mode && (mode == "none" || config.HasUser(owner))
}

func closeSocket(connection *websocket.Conn, code int, reason string) {
	// RFC 6455 leaves 123 bytes for the reason after the status code.
	if len(reason) > 123 {
		reason = reason[:123]
		for !utf8.ValidString(reason) {
			reason = reason[:len(reason)-1]
		}
	}
	_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(time.Second))
	_ = connection.Close()
}
func rejectSocket(connection *websocket.Conn, message string) {
	_ = connection.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = connection.WriteJSON(session.Event{"type": "error", "message": message})
	closeSocket(connection, 1008, message)
}

func (s *Server) terminalSocket(w http.ResponseWriter, r *http.Request) {
	// Register before upgrading so shutdown also waits for in-flight handshakes.
	s.socketMu.Lock()
	if s.closing {
		s.socketMu.Unlock()
		writeError(w, 503, "Server is shutting down")
		return
	}
	s.socketWorkers.Add(1)
	s.socketMu.Unlock()
	defer s.socketWorkers.Done()
	// The default origin check accepts same-host browser origins and non-browser
	// clients without Origin. Compression remains disabled, as in the Node server.
	upgrader := websocket.Upgrader{HandshakeTimeout: 5 * time.Second, ReadBufferSize: 4096, WriteBufferSize: 4096}
	connection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer connection.Close()
	s.socketMu.Lock()
	if s.closing {
		s.socketMu.Unlock()
		return
	}
	if s.sockets == nil {
		s.sockets = map[*websocket.Conn]struct{}{}
	}
	s.sockets[connection] = struct{}{}
	s.socketMu.Unlock()
	defer func() { s.socketMu.Lock(); delete(s.sockets, connection); s.socketMu.Unlock() }()
	owner, mode, ok := s.socketIdentity(r)
	if !ok {
		rejectSocket(connection, "Unauthorized")
		return
	}
	id := r.PathValue("id")
	viewer, err := s.sessions.Join(owner, id)
	if err != nil {
		var access *agent.AccessError
		if errors.As(err, &access) {
			rejectSocket(connection, access.Message)
			if access.Status != 500 {
				if cleanupErr := s.sessions.EnforceAgentAccess(); cleanupErr != nil {
					s.logger.Warn("clean up denied agent session", "error", cleanupErr)
				}
			}
			return
		}
		message := "Session not found"
		if !errors.Is(err, session.ErrNotFound) {
			message = "Session unavailable"
		}
		rejectSocket(connection, message)
		return
	}
	var toggles sync.WaitGroup
	toggleSlots := make(chan struct{}, 16)
	connection.SetReadLimit(1 << 20)
	writerDone := make(chan struct{})
	stopWriter := make(chan struct{})
	go func() {
		defer close(writerDone)
		defer connection.Close()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-viewer.Done():
				code, reason := viewer.CloseReason()
				closeSocket(connection, code, reason)
				return
			default:
			}
			select {
			case <-stopWriter:
				return
			case <-viewer.Done():
				code, reason := viewer.CloseReason()
				closeSocket(connection, code, reason)
				return
			case <-ticker.C:
				if !s.socketAuthorized(owner, mode) {
					closeSocket(connection, 1008, "Unauthorized")
					return
				}
			case <-viewer.Ready():
				event := viewer.Take()
				payload, err := json.Marshal(event)
				if err != nil {
					return
				}
				if err := connection.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
					return
				}
				// WriteMessage sends the complete JSON value in one frame even
				// when PTY output exceeds the connection's small write buffer.
				if err := connection.WriteMessage(websocket.TextMessage, payload); err != nil {
					return
				}
			}
		}
	}()
	defer func() {
		close(stopWriter)
		_ = connection.Close()
		s.sessions.Leave(owner, id, viewer.ID)
		<-writerDone
		toggles.Wait()
	}()
	for {
		kind, payload, err := connection.ReadMessage()
		if err != nil {
			return
		}
		if kind == websocket.TextMessage && !utf8.Valid(payload) {
			closeSocket(connection, 1007, "Invalid UTF-8")
			return
		}
		if !s.socketAuthorized(owner, mode) {
			closeSocket(connection, 1008, "Unauthorized")
			return
		}
		var message struct {
			Type string `json:"type"`
			Data string `json:"data"`
			Cols int    `json:"cols"`
			Rows int    `json:"rows"`
		}
		if json.Unmarshal(payload, &message) != nil {
			continue
		}
		switch message.Type {
		case "input":
			if message.Data != "" {
				if err := s.sessions.Input(owner, id, message.Data); errors.Is(err, session.ErrInputBusy) {
					closeSocket(connection, 1013, "Terminal input is busy")
					return
				}
			}
		case "resize":
			if message.Cols > 0 && message.Cols <= 500 && message.Rows > 0 && message.Rows <= 200 {
				_ = s.sessions.Resize(owner, id, message.Cols, message.Rows)
			}
		case "focus":
			_ = s.sessions.Focus(owner, id, viewer.ID)
		case "transcript_toggle":
			select {
			case toggleSlots <- struct{}{}:
			default:
				closeSocket(connection, 1013, "Transcript control is busy")
				return
			}
			toggles.Add(1)
			go func() {
				defer toggles.Done()
				defer func() { <-toggleSlots }()
				_ = s.sessions.ToggleTranscript(owner, id, viewer.ID)
			}()
		}
	}
}
