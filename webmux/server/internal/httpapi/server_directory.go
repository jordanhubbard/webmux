package httpapi

import (
	"errors"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/jordanhubbard/webmux/server/internal/storage"
)

var serverEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func serverCredential(env, saved string) string {
	if saved != "" {
		return saved
	}
	return os.Getenv(env)
}

func validServerCredential(env, saved string) bool {
	if saved == "" {
		return serverEnvName.MatchString(env)
	}
	return env == "" && validSavedCredential(saved)
}

func validSavedCredential(value string) bool {
	if len(value) < 32 || len(value) > 4096 {
		return false
	}
	for _, c := range value {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}

// Explicit response types keep credentials out of every directory read API.
type managedPeer struct {
	CoordinationMessages bool         `json:"coordination_messages,omitempty"`
	CoordinationOwner    string       `json:"coordination_owner,omitempty"`
	ID                   string       `json:"id"`
	Name                 string       `json:"name"`
	URL                  string       `json:"url"`
	AllowHTTP            bool         `json:"allow_http"`
	TokenEnv             string       `json:"token_env"`
	CredentialReady      bool         `json:"credential_ready"`
	Identity             nodeIdentity `json:"identity"`
}
type managedGrant struct {
	CoordinationMessages bool   `json:"coordination_messages,omitempty"`
	SourceNode           string `json:"source_node_id,omitempty"`
	SourceOwner          string `json:"source_owner,omitempty"`
	Coordination         bool   `json:"coordination,omitempty"`
	ID                   string `json:"id"`
	Owner                string `json:"owner"`
	TokenEnv             string `json:"token_env"`
	CredentialReady      bool   `json:"credential_ready"`
}

func (s *Server) getServerDirectory(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.federationAccess(w, owner) {
		return
	}
	d, err := s.serverDirectory()
	if err != nil {
		writeError(w, 500, "Cannot read servers.yaml; correct its configuration before managing servers")
		return
	}
	peers := []managedPeer{}
	grants := []managedGrant{}
	for _, p := range d.Servers {
		peers = append(peers, managedPeer{CoordinationMessages: p.CoordinationMessages, CoordinationOwner: p.CoordinationOwner, ID: p.ID, Name: p.Name, URL: p.URL, AllowHTTP: p.AllowHTTP, TokenEnv: p.TokenEnv, CredentialReady: validSavedCredential(serverCredential(p.TokenEnv, p.Credential)), Identity: p.Identity})
	}
	for _, g := range d.Grants {
		grants = append(grants, managedGrant{CoordinationMessages: g.CoordinationMessages, SourceNode: g.SourceNode, SourceOwner: g.SourceOwner, Coordination: g.Coordination, ID: g.ID, Owner: g.Owner, TokenEnv: g.TokenEnv, CredentialReady: validSavedCredential(serverCredential(g.TokenEnv, g.Credential))})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"servers": peers, "grants": grants})
}

// A blank credential preserves the existing source when token_env is unchanged.
// Supplying a credential switches to private, file-backed storage atomically.
func updateServerCredential(env, credential string, oldEnv, oldCredential string) (string, string) {
	if credential != "" {
		return "", credential
	}
	if env == oldEnv {
		return env, oldCredential
	}
	return env, ""
}

type directoryValidation struct{ error }

func (s *Server) changeServerDirectory(w http.ResponseWriter, change func(*serverDirectory) error) bool {
	err := storage.UpdateConfigOrCreate(s.store, "servers.yaml", serverDirectory{Version: 1}, func(d *serverDirectory) error {
		if err := change(d); err != nil {
			return directoryValidation{err}
		}
		if err := validateServerDirectory(*d); err != nil {
			return directoryValidation{err}
		}
		return nil
	})
	if err != nil {
		var validation directoryValidation
		if errors.As(err, &validation) {
			writeError(w, 400, validation.Error())
		} else {
			writeError(w, 500, "Cannot save servers.yaml; check the server configuration file")
		}
		return false
	}
	w.WriteHeader(http.StatusNoContent)
	return true
}

func (s *Server) saveServerPeer(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.federationAccess(w, owner) {
		return
	}
	var body struct {
		CoordinationMessages *bool   `json:"coordination_messages"`
		CoordinationOwner    *string `json:"coordination_owner"`
		Name                 string  `json:"name"`
		URL                  string  `json:"url"`
		AllowHTTP            bool    `json:"allow_http"`
		TokenEnv             string  `json:"token_env"`
		Credential           string  `json:"credential"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Credential != "" && !validSavedCredential(body.Credential) {
		writeError(w, 400, "Credential must contain 32–4096 printable ASCII characters without spaces")
		return
	}
	if body.CoordinationOwner != nil && *body.CoordinationOwner != "" && !s.federationOwnerAllowed(*body.CoordinationOwner) {
		writeError(w, 400, "Coordination mapping must name an existing local account")
		return
	}
	s.changeServerDirectory(w, func(d *serverDirectory) error {
		p := serverPeer{ID: r.PathValue("server"), Name: strings.TrimSpace(body.Name), URL: strings.TrimSpace(body.URL), AllowHTTP: body.AllowHTTP, TokenEnv: body.TokenEnv, Credential: body.Credential}
		if body.CoordinationOwner != nil {
			p.CoordinationOwner = *body.CoordinationOwner
		}
		if body.CoordinationMessages != nil {
			p.CoordinationMessages = *body.CoordinationMessages
		}
		for i, old := range d.Servers {
			if old.ID == p.ID {
				p.TokenEnv, p.Credential = updateServerCredential(body.TokenEnv, body.Credential, old.TokenEnv, old.Credential)
				if body.CoordinationMessages == nil && (body.CoordinationOwner == nil || *body.CoordinationOwner == old.CoordinationOwner) {
					p.CoordinationMessages = old.CoordinationMessages
				}
				p.Identity = old.Identity
				if body.CoordinationOwner == nil {
					p.CoordinationOwner = old.CoordinationOwner
				}
				if p.URL == old.URL {
					p.DiscoveredAddress = old.DiscoveredAddress
				}
				d.Servers[i] = p
				return nil
			}
		}
		if p.Credential != "" {
			p.TokenEnv = ""
		}
		d.Servers = append(d.Servers, p)
		return nil
	})
}

func (s *Server) deleteServerPeer(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.federationAccess(w, owner) {
		return
	}
	s.changeServerDirectory(w, func(d *serverDirectory) error {
		for i, p := range d.Servers {
			if p.ID == r.PathValue("server") {
				d.Servers = append(d.Servers[:i], d.Servers[i+1:]...)
				return nil
			}
		}
		return errors.New("Server is no longer configured; refresh the directory")
	})
}

func (s *Server) saveServerGrant(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.federationAccess(w, owner) {
		return
	}
	var body struct {
		CoordinationMessages *bool   `json:"coordination_messages"`
		SourceNode           *string `json:"source_node_id"`
		SourceOwner          *string `json:"source_owner"`
		Coordination         *bool   `json:"coordination"`
		Owner                string  `json:"owner"`
		TokenEnv             string  `json:"token_env"`
		Credential           string  `json:"credential"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if !s.federationOwnerAllowed(body.Owner) {
		writeError(w, 400, "Owner must be an existing account on this server (anonymous in trusted mode)")
		return
	}
	if body.Credential != "" && !validSavedCredential(body.Credential) {
		writeError(w, 400, "Credential must contain 32–4096 printable ASCII characters without spaces")
		return
	}
	s.changeServerDirectory(w, func(d *serverDirectory) error {
		g := serverGrant{ID: r.PathValue("grant"), Owner: body.Owner, TokenEnv: body.TokenEnv, Credential: body.Credential}
		if body.Coordination != nil {
			g.Coordination = *body.Coordination
		}
		if body.CoordinationMessages != nil {
			g.CoordinationMessages = *body.CoordinationMessages
		}
		if body.SourceNode != nil {
			g.SourceNode = *body.SourceNode
		}
		if body.SourceOwner != nil {
			g.SourceOwner = *body.SourceOwner
		}
		for i, old := range d.Grants {
			if old.ID == g.ID {
				same := old.Owner == g.Owner && (body.SourceNode == nil || *body.SourceNode == old.SourceNode) && (body.SourceOwner == nil || *body.SourceOwner == old.SourceOwner)
				if body.SourceNode == nil {
					g.SourceNode = old.SourceNode
				}
				if body.SourceOwner == nil {
					g.SourceOwner = old.SourceOwner
				}
				if body.CoordinationMessages == nil && same {
					g.CoordinationMessages = old.CoordinationMessages
				}
				if body.Coordination != nil && !*body.Coordination {
					g.CoordinationMessages = false
				}
				if body.Coordination == nil && old.Owner == g.Owner {
					g.Coordination = old.Coordination
				}
				g.TokenEnv, g.Credential = updateServerCredential(body.TokenEnv, body.Credential, old.TokenEnv, old.Credential)
				d.Grants[i] = g
				return nil
			}
		}
		if g.Credential != "" {
			g.TokenEnv = ""
		}
		d.Grants = append(d.Grants, g)
		return nil
	})
}

func (s *Server) deleteServerGrant(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.federationAccess(w, owner) {
		return
	}
	s.changeServerDirectory(w, func(d *serverDirectory) error {
		for i, g := range d.Grants {
			if g.ID == r.PathValue("grant") {
				d.Grants = append(d.Grants[:i], d.Grants[i+1:]...)
				return nil
			}
		}
		return errors.New("Grant is no longer configured; refresh the directory")
	})
}
