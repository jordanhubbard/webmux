package maccoord

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

const RuntimeSchema = "webmux.mac.observer.v1"

var ErrPolicy = errors.New("MAC observer configuration is unavailable or changed; reconcile configuration before continuing")

type credentialReference struct {
	AuthorityID    string        `json:"authority_id"`
	ClientID       string        `json:"client_id"`
	EnrollmentFile string        `json:"enrollment_file"`
	Publisher      *publisherPin `json:"publisher,omitempty"`
}

type publisherPin struct {
	PID         int    `json:"pid"`
	Fingerprint string `json:"fingerprint"`
}

// Runtime holds one accepted operator policy, never a response or credential
// cache. Edits fence reads and backend selection until the owner reconciles and
// reopens it. Tokens are read only for the selected receiving-account binding.
type Runtime struct {
	path        string
	digest      [32]byte
	configured  bool
	loadError   bool
	bindings    *Bindings
	credentials map[string]credentialReference
	roots       *x509.CertPool
}

// OpenRuntime does not interrupt terminal startup on invalid optional policy.
// Its methods then fail closed. A missing policy preserves standalone mode.
// roots is trusted embedding configuration; nil uses normal system TLS roots.
func OpenRuntime(path string, roots *x509.CertPool) *Runtime {
	r := &Runtime{path: path}
	if roots != nil {
		r.roots = roots.Clone()
	}
	if !filepath.IsAbs(path) {
		r.loadError = true
		return r
	}
	data, err := privateFile(path, 128<<10)
	if errors.Is(err, os.ErrNotExist) {
		return r
	}
	if err != nil {
		r.loadError = true
		return r
	}
	r.configured, r.digest = true, sha256.Sum256(data)
	r.bindings, r.credentials, err = parseRuntime(data)
	r.loadError = err != nil
	return r
}

func (r *Runtime) current() error {
	if r == nil || r.loadError {
		return ErrPolicy
	}
	data, err := privateFile(r.path, 128<<10)
	if !r.configured && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !r.configured || sha256.Sum256(data) != r.digest {
		return ErrPolicy
	}
	return nil
}

// Backend reports mac, standalone or an error requiring resolution. An unknown
// repository for a MAC-configured owner, or drift of a known remote/root, must
// not silently select a standalone authority. It does not acquire any lease.
func (r *Runtime) Backend(owner string, repository *coordination.Binding) (string, error) {
	if err := r.current(); err != nil {
		return "", err
	}
	return r.AcceptedBackend(owner, repository)
}

// AcceptedBackend is pure policy selection for admission transactions. The
// accepted execution policy changes only during a coordinated runtime reopen.
func (r *Runtime) AcceptedBackend(owner string, repository *coordination.Binding) (string, error) {
	if r == nil || r.loadError {
		return "", ErrPolicy
	}
	if !r.configured {
		return "standalone", nil
	}
	ownerConfigured := false
	for _, project := range r.bindings.projects {
		if project.Owner == owner {
			ownerConfigured = true
		}
	}
	if !ownerConfigured {
		return "standalone", nil
	}
	if repository == nil || !validText(repository.ID, 256) || !validText(repository.Repository, 2048) || (repository.Source != "verified-git" && repository.Source != "explicit-map") {
		return "", ErrUnmapped
	}
	if _, err := r.bindings.Resolve(owner, repository); err == nil {
		return "mac", nil
	}
	for _, project := range r.bindings.projects {
		if project.Owner == owner && (project.RepositoryID == repository.ID || project.Repository == repository.Repository) {
			return "", ErrUnmapped
		}
	}
	return "standalone", nil
}

// ObserveSession applies the same accepted policy to broker inventory. It does
// not register a MAC worker or infer a task/lease link from session presence.
func (r *Runtime) ObserveSession(owner string, member coordination.Member) (SessionObservation, error) {
	if mode, err := r.Backend(owner, member.Project); err != nil || mode != "mac" {
		return SessionObservation{}, ErrUnmapped
	}
	return r.bindings.ObserveSession(owner, member)
}

// ObserveSource is only for a worktree verified on this execution host. The
// caller must not pass a remote path to the local Git inspector. This observes
// Git metadata; it never establishes a task/attempt/source-roadmap association.
func (r *Runtime) ObserveSource(ctx context.Context, owner string, repository *coordination.Binding) (SourceObservation, error) {
	if mode, err := r.Backend(owner, repository); err != nil || mode != "mac" {
		return SourceObservation{}, ErrUnmapped
	}
	source, err := r.bindings.ObserveSource(ctx, owner, repository)
	if err != nil {
		return SourceObservation{}, err
	}
	if err := r.current(); err != nil {
		return SourceObservation{}, err
	}
	return source, nil
}

func (r *Runtime) ObserveRoadmap(ctx context.Context, owner string, repository *coordination.Binding, source SourceObservation) (RoadmapObservation, error) {
	if mode, err := r.Backend(owner, repository); err != nil || mode != "mac" {
		return RoadmapObservation{}, ErrUnmapped
	}
	roadmap, err := r.bindings.ObserveRoadmap(ctx, owner, repository, source)
	if err != nil {
		return RoadmapObservation{}, err
	}
	if err := r.current(); err != nil {
		return RoadmapObservation{}, err
	}
	return roadmap, nil
}

func (r *Runtime) ObserveProject(ctx context.Context, owner string, repository *coordination.Binding, offset, limit int) (ProjectObservation, error) {
	mode, err := r.Backend(owner, repository)
	if err != nil {
		return ProjectObservation{}, err
	}
	if mode != "mac" {
		return ProjectObservation{}, ErrUnmapped
	}
	if ctx == nil || offset < 0 || offset > 200 || limit < 1 || limit > ProjectPageLimit {
		return ProjectObservation{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	selected, err := r.bindings.Resolve(owner, repository)
	if err != nil {
		return ProjectObservation{}, err
	}
	client, err := r.openReadClient(ctx, selected)
	if err != nil {
		return ProjectObservation{}, err
	}
	view, err := r.bindings.ObserveProject(ctx, owner, repository, client.token, r.roots, offset, limit)
	if err != nil {
		return ProjectObservation{}, err
	}
	if err := r.finishReadClient(ctx, client); err != nil {
		return ProjectObservation{}, err
	}
	return view, nil
}

type runtimeReadClient struct {
	selected  Selection
	reference credentialReference
	token     string
	digest    [32]byte
}

func (r *Runtime) openReadClient(ctx context.Context, selected Selection) (runtimeReadClient, error) {
	var empty runtimeReadClient
	reference, exists := r.credentials[selected.Authority.ID]
	if !exists {
		return empty, ErrReadAccess
	}
	if err := qualifyPublisher(ctx, selected.Authority.Origin, reference.Publisher); err != nil {
		return empty, err
	}
	data, err := privateFile(reference.EnrollmentFile, 64<<10)
	if err != nil {
		return empty, ErrReadAccess
	}
	token, err := enrollmentToken(data, reference.ClientID, selected.Authority.Origin)
	if err != nil {
		return empty, err
	}
	// No business operation exists at this path. Actual generic write refusal
	// is required; a metadata label alone cannot qualify a read-only credential.
	if err := readOnlyCredential(ctx, selected.Authority.Origin, token, r.roots); err != nil {
		return empty, err
	}
	return runtimeReadClient{selected: selected, reference: reference, token: token, digest: sha256.Sum256(data)}, nil
}

func (r *Runtime) finishReadClient(ctx context.Context, client runtimeReadClient) error {
	if err := r.current(); err != nil {
		return err
	}
	after, err := privateFile(client.reference.EnrollmentFile, 64<<10)
	if err != nil || sha256.Sum256(after) != client.digest {
		return ErrReadAccess
	}
	// An unchanged file may have expired while local source data was sampled.
	token, err := enrollmentToken(after, client.reference.ClientID, client.selected.Authority.Origin)
	if err != nil || token != client.token {
		return ErrReadAccess
	}
	return qualifyPublisher(ctx, client.selected.Authority.Origin, client.reference.Publisher)
}

// CheckReadAccess is a final fresh qualification after local source/roadmap
// collection, so later stages cannot release earlier authorized task data after
// an observed upstream credential refusal. It retains no successful response.
func (r *Runtime) CheckReadAccess(ctx context.Context, owner string, repository *coordination.Binding) (ReadEvidence, error) {
	var empty ReadEvidence
	mode, err := r.Backend(owner, repository)
	if err != nil {
		return empty, err
	}
	if mode != "mac" {
		return empty, ErrUnmapped
	}
	if ctx == nil {
		return empty, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	selected, err := r.bindings.Resolve(owner, repository)
	if err != nil {
		return empty, err
	}
	client, err := r.openReadClient(ctx, selected)
	if err != nil {
		return empty, err
	}
	evidence, err := r.bindings.CheckReadAccess(ctx, owner, repository, client.token, r.roots)
	if err != nil {
		return empty, err
	}
	if err := r.finishReadClient(ctx, client); err != nil {
		return empty, err
	}
	return evidence, nil
}

func parseRuntime(data []byte) (*Bindings, map[string]credentialReference, error) {
	if !uniqueJSON(data) {
		return nil, nil, ErrInvalid
	}
	fields, ok := policyObject(data, "schema", "bindings", "credentials")
	if !ok {
		return nil, nil, ErrInvalid
	}
	var schema string
	if json.Unmarshal(fields["schema"], &schema) != nil || schema != RuntimeSchema {
		return nil, nil, ErrUnsupported
	}
	bindings, err := ParseBindings(fields["bindings"])
	if err != nil {
		return nil, nil, err
	}
	var entries []json.RawMessage
	if json.Unmarshal(fields["credentials"], &entries) != nil || entries == nil || len(entries) > 32 {
		return nil, nil, ErrInvalid
	}
	references := map[string]credentialReference{}
	for _, entry := range entries {
		fields, ok := policyObject(entry, "authority_id", "client_id", "enrollment_file", "publisher")
		if !ok {
			return nil, nil, ErrInvalid
		}
		if raw, exists := fields["publisher"]; exists {
			if _, ok := policyObject(raw, "pid", "fingerprint"); !ok {
				return nil, nil, ErrInvalid
			}
		}
		var reference credentialReference
		if json.Unmarshal(entry, &reference) != nil || !validText(reference.ClientID, 256) || !strings.HasPrefix(reference.ClientID, "webmux-") ||
			!filepath.IsAbs(reference.EnrollmentFile) || !validText(reference.EnrollmentFile, 4096) {
			return nil, nil, ErrInvalid
		}
		if _, exists := bindings.authorities[reference.AuthorityID]; !exists {
			return nil, nil, ErrInvalid
		}
		if _, exists := references[reference.AuthorityID]; exists {
			return nil, nil, ErrInvalid
		}
		if pin := reference.Publisher; pin != nil {
			decoded, err := hex.DecodeString(pin.Fingerprint)
			if pin.PID < 1 || err != nil || len(decoded) != 32 || strings.ToLower(pin.Fingerprint) != pin.Fingerprint {
				return nil, nil, ErrInvalid
			}
		}
		references[reference.AuthorityID] = reference
	}
	return bindings, references, nil
}

func policyObject(data []byte, names ...string) (map[string]json.RawMessage, bool) {
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

func enrollmentToken(data []byte, clientID, origin string) (string, error) {
	if !uniqueJSON(data) {
		return "", ErrReadAccess
	}
	var envelope, connection, credential map[string]json.RawMessage
	if json.Unmarshal(data, &envelope) != nil || json.Unmarshal(envelope["connection"], &connection) != nil || json.Unmarshal(envelope["credential"], &credential) != nil {
		return "", ErrReadAccess
	}
	schema, schemaOK := recordText(envelope, "schema", 128)
	id, idOK := recordText(envelope, "client_id", 256)
	apiURL, urlOK := recordText(connection, "api_url", 2048)
	token, tokenOK := recordText(credential, "token", 16384)
	expires, expiresOK := recordTime(credential, "expires_at")
	var scopes []string
	if !schemaOK || schema != "mac.client_enrollment.v1" || !idOK || id != clientID || !urlOK || strings.TrimSuffix(apiURL, "/") != origin ||
		!tokenOK || !validReadToken(token) || !expiresOK || !expires.After(time.Now()) || json.Unmarshal(credential["scopes"], &scopes) != nil || len(scopes) != 1 || scopes[0] != "read" {
		return "", ErrReadAccess
	}
	return token, nil
}

func privateFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, os.ErrNotExist
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, ErrInvalid
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrInvalid
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !privateOwnerFile(path, opened) {
		return nil, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit || !json.Valid(data) {
		return nil, ErrInvalid
	}
	return data, nil
}

func readOnlyCredential(parent context.Context, origin, token string, roots *x509.CertPool) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	if roots != nil {
		config.RootCAs = roots.Clone()
	}
	transport := &http.Transport{Proxy: nil, TLSClientConfig: config, DisableKeepAlives: true, MaxResponseHeaderBytes: 16 << 10}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+"/__webmux_read_scope_probe_no_handler__", bytes.NewReader(nil))
	if err != nil {
		return ErrReadAccess
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		return &UnavailableError{canceled: ctx.Err() == context.Canceled, timedOut: ctx.Err() == context.DeadlineExceeded}
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		return ErrReadAccess
	}
	return nil
}
