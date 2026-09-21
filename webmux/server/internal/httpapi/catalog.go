package httpapi

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/jordanhubbard/webmux/server/internal/storage"
)

// Catalog records are extensible YAML objects. Preserve missing fields, nulls,
// and operator-defined metadata rather than rewriting older files to a new
// schema. Incoming known fields are checked with the typed request structures.
type catalogRecord map[string]any

type hostsConfig struct {
	Hosts []catalogRecord `yaml:"hosts"`
	Extra map[string]any  `yaml:",inline"`
}

type keysConfig struct {
	Keys  []catalogRecord `yaml:"keys"`
	Extra map[string]any  `yaml:",inline"`
}

type hostInput struct {
	ID          string   `json:"id"`
	Hostname    string   `json:"hostname"`
	Port        int      `json:"port"`
	Username    string   `json:"username"`
	Transport   string   `json:"transport"`
	KeyID       string   `json:"key_id"`
	Tags        []string `json:"tags"`
	MoshAllowed bool     `json:"mosh_allowed"`
	VNCEnabled  bool     `json:"vnc_enabled"`
	VNCPort     *int     `json:"vnc_port"`
	RDPEnabled  bool     `json:"rdp_enabled"`
	RDPPort     *int     `json:"rdp_port"`
}

type keyInput struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	PrivateKeyPath string `json:"private_key_path"`
	Encrypted      bool   `json:"encrypted"`
	Description    string `json:"description"`
}

func uuid() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:]), nil
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func defaultPort(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func (s *Server) catalogError(w http.ResponseWriter, err error, message string) {
	var response *responseError
	if errors.As(err, &response) {
		writeError(w, response.status, response.message)
		return
	}
	s.logger.Error(message, "error", err)
	writeError(w, 500, message)
}

func (s *Server) listHosts(w http.ResponseWriter, r *http.Request, _ string) {
	var config hostsConfig
	if err := s.store.ReadConfig("hosts.yaml", &config); err != nil {
		s.catalogError(w, err, "Failed to load hosts")
		return
	}
	writeJSON(w, 200, config.Hosts)
}

func (s *Server) createHost(w http.ResponseWriter, r *http.Request, _ string) {
	var body hostInput
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Hostname == "" {
		writeError(w, 400, "hostname is required")
		return
	}
	id := body.ID
	if id == "" {
		var err error
		id, err = uuid()
		if err != nil {
			s.catalogError(w, err, "Failed to save host")
			return
		}
	}
	port := body.Port
	if port == 0 {
		port = 22
	}
	if body.Tags == nil {
		body.Tags = []string{}
	}
	host := catalogRecord{
		"id": id, "hostname": body.Hostname, "port": port,
		"username": body.Username, "transport": defaultString(body.Transport, "ssh"),
		"key_id": body.KeyID, "tags": body.Tags, "mosh_allowed": body.MoshAllowed,
		"vnc_enabled": body.VNCEnabled, "vnc_port": defaultPort(body.VNCPort, 5900),
		"rdp_enabled": body.RDPEnabled, "rdp_port": defaultPort(body.RDPPort, 3389),
	}
	err := storage.UpdateConfig(s.store, "hosts.yaml", func(config *hostsConfig) error {
		if config.Hosts == nil {
			return errors.New("hosts must be an array")
		}
		config.Hosts = append(config.Hosts, host)
		return nil
	})
	if err != nil {
		s.catalogError(w, err, "Failed to save host")
		return
	}
	writeJSON(w, 201, host)
}

func (s *Server) updateHost(w http.ResponseWriter, r *http.Request, _ string) {
	var updates catalogRecord
	if !decodeJSON(w, r, &updates) {
		return
	}
	if updates == nil {
		updates = catalogRecord{}
	}
	encoded, err := json.Marshal(updates)
	if err != nil {
		jsonError(w, err)
		return
	}
	var checked hostInput
	if err := json.Unmarshal(encoded, &checked); err != nil {
		jsonError(w, err)
		return
	}
	id := r.PathValue("id")
	var updated catalogRecord
	err = storage.UpdateConfig(s.store, "hosts.yaml", func(config *hostsConfig) error {
		for _, host := range config.Hosts {
			if host["id"] != id {
				continue
			}
			for key, value := range updates {
				host[key] = value
			}
			host["id"] = id
			if host["vnc_enabled"] == nil {
				host["vnc_enabled"] = false
			}
			if host["vnc_port"] == nil {
				host["vnc_port"] = 5900
			}
			updated = host
			return nil
		}
		return &responseError{404, "Host not found"}
	})
	if err != nil {
		s.catalogError(w, err, "Failed to update host")
		return
	}
	writeJSON(w, 200, updated)
}

func (s *Server) deleteHost(w http.ResponseWriter, r *http.Request, _ string) {
	id := r.PathValue("id")
	err := storage.UpdateConfig(s.store, "hosts.yaml", func(config *hostsConfig) error {
		for i, host := range config.Hosts {
			if host["id"] == id {
				config.Hosts = append(config.Hosts[:i], config.Hosts[i+1:]...)
				return nil
			}
		}
		return &responseError{404, "Host not found"}
	})
	if err != nil {
		s.catalogError(w, err, "Failed to delete host")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func publicKey(key catalogRecord) catalogRecord {
	result := make(catalogRecord)
	for _, field := range []string{"id", "type", "encrypted", "description"} {
		if value, ok := key[field]; ok {
			result[field] = value
		}
	}
	return result
}

func (s *Server) listKeys(w http.ResponseWriter, r *http.Request, _ string) {
	var config keysConfig
	if err := s.store.ReadConfig("keys.yaml", &config); err != nil {
		s.catalogError(w, err, "Failed to load keys")
		return
	}
	if config.Keys == nil {
		s.catalogError(w, errors.New("keys must be an array"), "Failed to load keys")
		return
	}
	keys := make([]catalogRecord, 0, len(config.Keys))
	for _, key := range config.Keys {
		keys = append(keys, publicKey(key))
	}
	writeJSON(w, 200, keys)
}

func (s *Server) createKey(w http.ResponseWriter, r *http.Request, _ string) {
	var body keyInput
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.PrivateKeyPath == "" {
		writeError(w, 400, "private_key_path is required")
		return
	}
	id := body.ID
	if id == "" {
		var err error
		id, err = uuid()
		if err != nil {
			s.catalogError(w, err, "Failed to save key")
			return
		}
	}
	key := catalogRecord{"id": id, "type": defaultString(body.Type, "rsa"), "private_key_path": body.PrivateKeyPath, "encrypted": body.Encrypted, "description": body.Description}
	err := storage.UpdateConfig(s.store, "keys.yaml", func(config *keysConfig) error {
		if config.Keys == nil {
			return errors.New("keys must be an array")
		}
		config.Keys = append(config.Keys, key)
		return nil
	})
	if err != nil {
		s.catalogError(w, err, "Failed to save key")
		return
	}
	writeJSON(w, 201, publicKey(key))
}

func (s *Server) deleteKey(w http.ResponseWriter, r *http.Request, _ string) {
	id := r.PathValue("id")
	err := storage.UpdateConfig(s.store, "keys.yaml", func(config *keysConfig) error {
		for i, key := range config.Keys {
			if key["id"] == id {
				config.Keys = append(config.Keys[:i], config.Keys[i+1:]...)
				return nil
			}
		}
		return &responseError{404, "Key not found"}
	})
	if err != nil {
		s.catalogError(w, err, "Failed to delete key")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
