package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/jordanhubbard/webmux/server/internal/browser"
	"github.com/jordanhubbard/webmux/server/internal/session"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

type savedBrowserProfile struct {
	ID    string `json:"id" yaml:"id"`
	Name  string `json:"name" yaml:"name"`
	Owner string `json:"-" yaml:"owner"`
	Scope string `json:"-" yaml:"scope"`
	Key   string `json:"-" yaml:"key"`
}
type browserProfileConfig struct {
	Profiles []savedBrowserProfile `yaml:"profiles"`
}

func browserProfileScope(s session.Session) (string, error) {
	var principal any
	if s.Transport == "local" || s.BrowserLocal {
		principal = []string{"local"}
	} else {
		if s.Transport != "ssh" && s.Transport != "mosh" && s.Transport != "" {
			return "", errors.New("Saved browser profiles require a Local, SSH or Mosh terminal")
		}
		port := s.Port
		if port == 0 {
			port = 22
		}
		principal = struct {
			Host, User string
			Port       int
		}{strings.ToLower(s.Hostname), s.Username, port}
	}
	data, _ := json.Marshal(principal)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
func (s *Server) readBrowserProfiles() (browserProfileConfig, error) {
	var config browserProfileConfig
	err := s.store.ReadConfig("browser-profiles.yaml", &config)
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return config, err
}
func (s *Server) selectedBrowserProfile(owner, scope, id string) (*browser.Profile, error) {
	if id == "" {
		return &browser.Profile{}, nil
	}
	config, err := s.readBrowserProfiles()
	if err != nil {
		return nil, err
	}
	for _, profile := range config.Profiles {
		if profile.ID == id && profile.Owner == owner && profile.Scope == scope {
			return &browser.Profile{ID: profile.ID, Name: profile.Name, Key: profile.Key}, nil
		}
	}
	return nil, &responseError{404, "Saved browser profile not found on this execution host"}
}

type profileOutput struct{ bytes.Buffer }

func (b *profileOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 4096 {
		return 0, errors.New("oversized profile response")
	}
	return b.Buffer.Write(p)
}
func (s *Server) browserProfiles(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "no-store")
	terminal, err := s.sessions.Get(owner, r.PathValue("id"))
	if err != nil {
		s.sessionError(w, err, "Terminal not found")
		return
	}
	scope, err := browserProfileScope(terminal)
	if err != nil {
		writeError(w, 409, err.Error())
		return
	}
	s.browserProfileMu.Lock()
	defer s.browserProfileMu.Unlock()
	if r.Method == http.MethodGet {
		config, err := s.readBrowserProfiles()
		if err != nil {
			s.internalError(w, err)
			return
		}
		values := []savedBrowserProfile{}
		for _, profile := range config.Profiles {
			if profile.Owner == owner && profile.Scope == scope {
				values = append(values, profile)
			}
		}
		writeJSON(w, 200, values)
		return
	}
	var body struct {
		Name string `json:"name"`
		ID   string `json:"profileId"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if r.Method == http.MethodPost {
		body.Name = strings.TrimSpace(body.Name)
		if body.Name == "" || len(body.Name) > 64 || strings.IndexFunc(body.Name, unicode.IsControl) >= 0 {
			writeError(w, 400, "Choose a profile name of 1 to 64 characters")
			return
		}
		id, err := uuid()
		if err != nil {
			s.internalError(w, err)
			return
		}
		key, err := randomClientValue(32)
		if err != nil {
			s.internalError(w, err)
			return
		}
		profile := savedBrowserProfile{ID: id, Key: key, Owner: owner, Scope: scope, Name: body.Name}
		err = storage.UpdateConfigOrCreate(s.store, "browser-profiles.yaml", browserProfileConfig{}, func(config *browserProfileConfig) error {
			owned := 0
			for _, p := range config.Profiles {
				if p.Owner == owner {
					owned++
				}
				if p.Owner == owner && p.Scope == scope && p.Name == body.Name {
					return &responseError{409, "A profile with this name already exists on this host"}
				}
			}
			if owned >= 32 || len(config.Profiles) >= 1024 {
				return &responseError{409, "Saved browser profile limit reached"}
			}
			config.Profiles = append(config.Profiles, profile)
			return nil
		})
		if err != nil {
			s.catalogError(w, err, "Cannot save browser profile")
			return
		}
		writeJSON(w, 201, profile)
		return
	}
	profile, err := s.selectedBrowserProfile(owner, scope, body.ID)
	if err != nil {
		s.catalogError(w, err, "Profile not found")
		return
	}
	if profile.ID == "" {
		writeError(w, 400, "Choose a saved browser profile to remove")
		return
	}
	if s.browsers.ProfileInUse(profile.Key) {
		writeError(w, 409, "End browsers using this profile before deleting it")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	cmd, err := browser.WorkerCommand(ctx, terminal, s.store, "--browser-profile-remove")
	if err != nil {
		writeError(w, 409, err.Error())
		return
	}
	request, _ := json.Marshal(browser.Request{ProfileKey: profile.Key})
	cmd.Stdin = bytes.NewReader(request)
	var output profileOutput
	cmd.Stdout = &output
	cmd.WaitDelay = time.Second
	err = cmd.Run()
	var reply browser.Frame
	if json.Unmarshal(output.Bytes(), &reply) != nil || err != nil || reply.Error != "" || reply.ProfileKey != profile.Key || reply.Protocol != browser.Protocol {
		writeError(w, 409, "Profile is in use, unavailable, or the terminal host needs matching WebMux. Its saved metadata was retained.")
		return
	}
	err = storage.UpdateConfig(s.store, "browser-profiles.yaml", func(config *browserProfileConfig) error {
		kept := config.Profiles[:0]
		for _, p := range config.Profiles {
			if p.ID != profile.ID || p.Owner != owner {
				kept = append(kept, p)
			}
		}
		config.Profiles = kept
		return nil
	})
	if err != nil {
		s.internalError(w, err)
		return
	}
	w.WriteHeader(204)
}
