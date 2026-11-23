// Package maccoord defines the MAC observation boundary. A configured mapping
// selects a read projection; it does not authenticate an authority, register a
// worker, grant a lease, or make a WebMux session a MAC participant.
package maccoord

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

const BindingsSchema = "webmux.mac.project-bindings.v1"
const ObserveCapability = "project.observe.v1"

var (
	ErrInvalid     = errors.New("invalid MAC observation contract")
	ErrUnsupported = errors.New("unsupported MAC observation schema or capability")
	ErrUnmapped    = errors.New("MAC observation binding is unavailable")
)

// Authority names an operator-selected origin. ID is scoped by that operator,
// not derived from a worker nickname or an unauthenticated advertisement.
// Credentials deliberately have no representation in this contract.
type Authority struct {
	ID     string `json:"id"`
	Origin string `json:"origin"`
}

// ProjectBinding must be configured by the server operator, never supplied by
// a browser or session request. Repository and RepositoryID are the exact
// canonical values from a verified execution-host resolver or explicit map.
// ProjectKey is MAC's authority-scoped task.project key, not an invented UUID.
type ProjectBinding struct {
	Owner        string   `json:"owner"`
	RepositoryID string   `json:"repository_id"`
	Repository   string   `json:"repository"`
	AuthorityID  string   `json:"authority_id"`
	ProjectKey   string   `json:"project_key"`
	Capabilities []string `json:"capabilities"`
}

type bindingDocument struct {
	Schema      string           `json:"schema"`
	Authorities []Authority      `json:"authorities"`
	Bindings    []ProjectBinding `json:"bindings"`
}

type bindingKey struct{ owner, repository string }

// Bindings is immutable after parsing. Its revision changes when any mapping
// changes, allowing later adapters to scope observations to their exact policy.
type Bindings struct {
	revision    string
	authorities map[string]Authority
	projects    map[bindingKey]ProjectBinding
}

// Selection is a copy. It remains unqualified for live access until a transport
// verifies the intended authority's actual authentication/capabilities.
type Selection struct {
	Revision  string
	Authority Authority
	Project   ProjectBinding
}

func validText(value string, max int) bool {
	return value != "" && len(value) <= max && utf8.ValidString(value) &&
		strings.TrimSpace(value) == value && !strings.ContainsRune(value, utf8.RuneError) &&
		!strings.ContainsFunc(value, unicode.IsControl)
}

func validOrigin(value string) bool {
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" ||
		(u.Path != "" && u.Path != "/") || u.RawPath != "" || strings.Contains(value, "#") {
		return false
	}
	if port := u.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return false
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	// Cleartext is allowed only for an explicitly selected loopback authority.
	if strings.EqualFold(u.Hostname(), "localhost") {
		return true
	}
	address, err := netip.ParseAddr(u.Hostname())
	return err == nil && address.IsLoopback()
}

// ParseBindings accepts a bounded, strict operator document. No environment,
// files, repository, network, credentials or process state are inspected.
func ParseBindings(data []byte) (*Bindings, error) {
	if len(data) > 64<<10 || !utf8.Valid(data) || !uniqueJSON(data) || !exactBindingFields(data) {
		return nil, ErrInvalid
	}
	var document bindingDocument
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&document) != nil {
		return nil, ErrInvalid
	}
	if document.Schema != BindingsSchema {
		return nil, ErrUnsupported
	}
	if len(document.Authorities) > 32 || len(document.Bindings) > 256 {
		return nil, ErrInvalid
	}
	out := &Bindings{authorities: map[string]Authority{}, projects: map[bindingKey]ProjectBinding{}}
	for i, authority := range document.Authorities {
		if !validText(authority.ID, 128) || !validText(authority.Origin, 2048) || !validOrigin(authority.Origin) {
			return nil, ErrInvalid
		}
		if _, duplicate := out.authorities[authority.ID]; duplicate {
			return nil, ErrInvalid
		}
		// Equivalent root URL spellings must not produce divergent revisions.
		authority.Origin = strings.TrimSuffix(authority.Origin, "/")
		document.Authorities[i] = authority
		out.authorities[authority.ID] = authority
	}
	for i, project := range document.Bindings {
		if !validText(project.Owner, 256) || !validText(project.RepositoryID, 256) ||
			!validText(project.Repository, 2048) || !validText(project.ProjectKey, 256) ||
			strings.ContainsAny(project.ProjectKey, "/?#"+string(rune(92))) {
			return nil, ErrInvalid
		}
		if _, exists := out.authorities[project.AuthorityID]; !exists {
			return nil, ErrInvalid
		}
		// v1 deliberately has no admission, dispatch, lease or publication
		// capability. An operator cannot enable unsupported operations by name.
		if len(project.Capabilities) != 1 || project.Capabilities[0] != ObserveCapability {
			return nil, ErrUnsupported
		}
		key := bindingKey{project.Owner, project.RepositoryID}
		if _, duplicate := out.projects[key]; duplicate {
			return nil, ErrInvalid
		}
		project.Capabilities = append([]string(nil), project.Capabilities...)
		document.Bindings[i] = project
		out.projects[key] = project
	}
	sort.Slice(document.Authorities, func(i, j int) bool { return document.Authorities[i].ID < document.Authorities[j].ID })
	sort.Slice(document.Bindings, func(i, j int) bool {
		a, b := document.Bindings[i], document.Bindings[j]
		if a.Owner != b.Owner {
			return a.Owner < b.Owner
		}
		return a.RepositoryID < b.RepositoryID
	})
	canonical, err := json.Marshal(document)
	if err != nil {
		return nil, ErrInvalid
	}
	digest := sha256.Sum256(canonical)
	out.revision = hex.EncodeToString(digest[:])
	return out, nil
}

// Resolve consumes only server-resolved repository observations. Callers must
// not turn client-supplied CWDs, names, filters or IDs into such observations.
// Failure deliberately reveals no other account's bindings or authority.
func (b *Bindings) Resolve(owner string, repository *coordination.Binding) (Selection, error) {
	if b == nil || repository == nil || !validText(owner, 256) ||
		(repository.Source != "verified-git" && repository.Source != "explicit-map") {
		return Selection{}, ErrUnmapped
	}
	project, exists := b.projects[bindingKey{owner, repository.ID}]
	if !exists || project.Repository != repository.Repository {
		return Selection{}, ErrUnmapped
	}
	project.Capabilities = append([]string(nil), project.Capabilities...)
	return Selection{Revision: b.revision, Authority: b.authorities[project.AuthorityID], Project: project}, nil
}

// encoding/json matches struct fields case-insensitively, including tagged
// fields. A versioned policy cannot silently treat "Owner" as "owner".
func exactBindingFields(data []byte) bool {
	object := func(data []byte, names ...string) (map[string]json.RawMessage, bool) {
		var fields map[string]json.RawMessage
		if json.Unmarshal(data, &fields) != nil || fields == nil {
			return nil, false
		}
		for key := range fields {
			known := false
			for _, name := range names {
				known = known || key == name
			}
			if !known {
				return nil, false
			}
		}
		return fields, true
	}
	document, ok := object(data, "schema", "authorities", "bindings")
	if !ok {
		return false
	}
	for _, section := range []struct {
		name   string
		fields []string
	}{
		{"authorities", []string{"id", "origin"}},
		{"bindings", []string{"owner", "repository_id", "repository", "authority_id", "project_key", "capabilities"}},
	} {
		raw := document[section.name]
		if len(raw) == 0 {
			continue
		}
		var rows []json.RawMessage
		if json.Unmarshal(raw, &rows) != nil {
			return false
		}
		for _, row := range rows {
			if _, ok := object(row, section.fields...); !ok {
				return false
			}
		}
	}
	return true
}

// Reject duplicate object keys, trailing values and excessive nesting before
// decoding the versioned document. Last-key-wins parsing is not a policy rule.
func uniqueJSON(data []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var value func(int) bool
	value = func(depth int) bool {
		if depth > 16 {
			return false
		}
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		delim, compound := token.(json.Delim)
		if !compound {
			return true
		}
		switch delim {
		case '{':
			keys := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				name, ok := key.(string)
				if err != nil || !ok || keys[name] || !value(depth+1) {
					return false
				}
				keys[name] = true
			}
			end, err := decoder.Token()
			return err == nil && end == json.Delim('}')
		case '[':
			for decoder.More() {
				if !value(depth + 1) {
					return false
				}
			}
			end, err := decoder.Token()
			return err == nil && end == json.Delim(']')
		}
		return false
	}
	if !value(0) {
		return false
	}
	_, err := decoder.Token()
	return errors.Is(err, io.EOF)
}
