package httpapi

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/desktop"
	"github.com/jordanhubbard/webmux/server/internal/guacamole"
	"github.com/jordanhubbard/webmux/server/internal/netguard"
)

func (s *Server) desktopIdentity(r *http.Request) (owner, mode string, ok bool) {
	cfg, err := auth.LoadConfig(s.store)
	if err != nil {
		return "", "", false
	}
	mode = cfg.Auth.Mode
	owner = "anonymous"
	if ticket := r.URL.Query().Get("ticket"); ticket != "" {
		owner, ok = s.auth.ConsumeTicket(ticket)
		if !ok {
			return "", mode, false
		}
	} else if token := r.URL.Query().Get("token"); mode == "local" || token != "" {
		subject, err := s.auth.Verify(token)
		if err != nil {
			if mode == "local" {
				return "", mode, false
			}
		} else {
			owner = subject
		}
	}
	return owner, mode, mode == "none" || cfg.HasUser(owner)
}

func (s *Server) vncSocket(w http.ResponseWriter, r *http.Request) { s.desktopSocket(w, r, false) }
func (s *Server) rdpSocket(w http.ResponseWriter, r *http.Request) { s.desktopSocket(w, r, true) }

func (s *Server) desktopSocket(w http.ResponseWriter, r *http.Request, rdp bool) {
	s.socketMu.Lock()
	if s.closing {
		s.socketMu.Unlock()
		writeError(w, 503, "Server is shutting down")
		return
	}
	s.socketWorkers.Add(1)
	s.socketMu.Unlock()
	defer s.socketWorkers.Done()
	upgrader := websocket.Upgrader{HandshakeTimeout: 5 * time.Second, ReadBufferSize: 4096, WriteBufferSize: 32768}
	if rdp {
		upgrader.Subprotocols = []string{"guacamole"}
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	s.socketMu.Lock()
	if s.closing {
		s.socketMu.Unlock()
		return
	}
	if s.sockets == nil {
		s.sockets = map[*websocket.Conn]struct{}{}
	}
	s.sockets[ws] = struct{}{}
	s.socketMu.Unlock()
	defer func() { s.socketMu.Lock(); delete(s.sockets, ws); s.socketMu.Unlock() }()
	owner, mode, ok := s.desktopIdentity(r)
	if !ok {
		closeSocket(ws, 1008, "Unauthorized")
		return
	}
	broker := s.vnc
	transportName := "TCP"
	if rdp {
		broker = s.rdp
		transportName = "guacd"
	}
	connection, err := broker.Connect(owner, r.PathValue("id"))
	if err != nil {
		message := "Session not found"
		if errors.Is(err, desktop.ErrForbidden) {
			message = "Forbidden"
		}
		closeSocket(ws, 1008, message)
		return
	}
	finalState := "disconnected"
	defer func() { connection.Finish(finalState) }()
	ctx, cancel := context.WithCancel(r.Context())
	var workers sync.WaitGroup
	// Cancel both directions before waiting, including clients closed during DNS.
	defer func() { cancel(); _ = ws.Close(); workers.Wait() }()
	messages := make(chan []byte, 4)
	var ready atomic.Bool
	ready.Store(!rdp)
	ws.SetReadLimit(1 << 20)
	workers.Go(func() {
		defer cancel()
		for {
			kind, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if !s.socketAuthorized(owner, mode) {
				closeSocket(ws, 1008, "Unauthorized")
				return
			}
			if (rdp || kind == websocket.TextMessage) && !utf8.Valid(data) {
				closeSocket(ws, 1007, "Invalid UTF-8")
				return
			}
			if !ready.Load() {
				continue
			}
			select {
			case messages <- data:
			case <-ctx.Done():
				return
			}
		}
	})
	workers.Go(func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-connection.Done:
				closeSocket(ws, 1000, "Session deleted")
				cancel()
				return
			case <-ticker.C:
				if !s.socketAuthorized(owner, mode) {
					closeSocket(ws, 1008, "Unauthorized")
					cancel()
					return
				}
			}
		}
	})
	target, err := s.targets.Resolve(ctx, connection.Session.Hostname)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, netguard.ErrHostname) {
			finalState = connection.Session.State
			closeSocket(ws, 1008, "Invalid hostname")
		} else {
			finalState = "error"
			closeSocket(ws, 1008, "Target blocked")
		}
		return
	}
	port := connection.Session.VNCPort
	if rdp {
		port = connection.Session.RDPPort
	}
	if port < 1 || port > 65535 {
		finalState = "error"
		closeSocket(ws, 1008, "Target blocked")
		return
	}
	address := net.JoinHostPort(target, strconv.Itoa(port))
	if rdp {
		address, err = s.guacdAddress()
		if err != nil {
			finalState = "error"
			closeSocket(ws, 1011, "guacd connection error")
			return
		}
	}
	tcp, err := (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		finalState = "error"
		closeSocket(ws, 1011, transportName+" connection error")
		return
	}
	defer tcp.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = tcp.Close() })
	defer stopClose()
	var decoder *guacamole.Decoder
	if rdp {
		decoder = guacamole.NewDecoder(tcp)
		_ = tcp.SetDeadline(time.Now().Add(10 * time.Second))
		parameters := guacamole.Parameters{Hostname: target, Port: port, Password: connection.Password}
		if connection.Session.RDPUsername != nil {
			parameters.Username = *connection.Session.RDPUsername
		}
		if connection.Session.RDPDomain != nil {
			parameters.Domain = *connection.Session.RDPDomain
		}
		instruction, err := guacamole.Handshake(decoder, tcp, parameters)
		parameters.Password = ""
		connection.Password = ""
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			finalState = "error"
			reason := "guacd connection error"
			if errors.Is(err, guacamole.ErrRemote) || errors.Is(err, guacamole.ErrProtocol) {
				reason = "guacd error"
			}
			closeSocket(ws, 1011, reason)
			return
		}
		_ = tcp.SetDeadline(time.Time{})
		// Accept replies as soon as ready becomes visible to the browser.
		ready.Store(true)
		_ = ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := ws.WriteMessage(websocket.TextMessage, []byte(instruction.Raw)); err != nil {
			return
		}
	}
	connection.SetState("connected")
	readResult := make(chan error, 1)
	workers.Go(func() {
		buffer := make([]byte, 32768)
		for {
			var payload []byte
			var err error
			messageType := websocket.BinaryMessage
			if rdp {
				var instruction guacamole.Instruction
				instruction, err = decoder.Read()
				if err == nil {
					payload = []byte(instruction.Raw)
				}
				messageType = websocket.TextMessage
			} else {
				var n int
				n, err = tcp.Read(buffer)
				payload = buffer[:n]
			}
			if len(payload) > 0 {
				_ = ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if writeErr := ws.WriteMessage(messageType, payload); writeErr != nil {
					readResult <- writeErr
					return
				}
			}
			if err != nil {
				readResult <- err
				return
			}
		}
	})
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-readResult:
			if errors.Is(err, io.EOF) {
				closeSocket(ws, 1001, transportName+" connection closed")
			} else {
				finalState = "error"
				closeSocket(ws, 1011, transportName+" connection error")
			}
			return
		case data := <-messages:
			_ = tcp.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := tcp.Write(data); err != nil {
				if ctx.Err() == nil {
					finalState = "error"
					closeSocket(ws, 1011, transportName+" connection error")
				}
				return
			}
		}
	}
}
