// Package credentialhub implements the opt-in account-scoped credential vault.
// It has no MAC dependency and never returns credential values through metadata.
package credentialhub

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

var (
	ErrDenied      = errors.New("credential access refused")
	ErrUnavailable = errors.New("credential hub unavailable; check private key custody and configuration")
	ErrLimit       = errors.New("credential hub limit reached")
)

const maxDocument = 2 << 20
const handleTTL = 30 * time.Second
const associatedData = "webmux.credential-hub.v1"

// Scope is derived from a current independently revocable incoming enrollment.
// Project is reserved: the initial AI consumers are strictly account-scoped.
type Scope struct {
	Owner                 string `json:"owner"`
	SecretID              string `json:"secret_id"`
	EnrollmentID          string `json:"enrollment_id"`
	EnrollmentFingerprint string `json:"enrollment_fingerprint"`
	SourceNode            string `json:"source_node"`
	SourceOwner           string `json:"source_owner"`
	Consumer              string `json:"consumer"`
	Project               string `json:"project"`
}
type Secret struct {
	ID       string `json:"id"`
	Owner    string `json:"owner"`
	Name     string `json:"name"`
	Consumer string `json:"consumer"`
	Version  uint64 `json:"version"`
	Enabled  bool   `json:"enabled"`
}
type Grant struct {
	Active                bool   `json:"active,omitempty"`
	EnrollmentFingerprint string `json:"-"`
	ID                    string `json:"id"`
	SecretID              string `json:"secret_id"`
	EnrollmentID          string `json:"enrollment_id"`
	SourceNode            string `json:"source_node"`
	SourceOwner           string `json:"source_owner"`
	Consumer              string `json:"consumer"`
}
type Audit struct {
	At           time.Time `json:"at"`
	Operation    string    `json:"operation"`
	SecretID     string    `json:"secret_id,omitempty"`
	EnrollmentID string    `json:"enrollment_id,omitempty"`
	Allowed      bool      `json:"allowed"`
}
type View struct {
	Enabled bool     `json:"enabled"`
	Secrets []Secret `json:"secrets"`
	Grants  []Grant  `json:"grants"`
	Audit   []Audit  `json:"audit"`
}
type storedSecret struct {
	Secret
	Value string `json:"value"`
}
type storedGrant struct {
	Grant
	Owner       string `json:"owner"`
	Fingerprint string `json:"fingerprint"`
}
type document struct {
	Version int             `json:"version"`
	Owners  map[string]bool `json:"owners"`
	Secrets []storedSecret  `json:"secrets"`
	Grants  []storedGrant   `json:"grants"`
	Audit   []struct {
		Audit
		Owner string `json:"owner"`
	} `json:"audit"`
}
type handle struct {
	Scope   Scope
	GrantID string
	Version uint64
	Expires time.Time
}
type Vault struct {
	mu      sync.Mutex
	dir     string
	handles map[[32]byte]handle
	now     func() time.Time
}

func New(home string) *Vault {
	return &Vault{dir: filepath.Join(home, "data", "credential-hub"), handles: make(map[[32]byte]handle), now: time.Now}
}
func randomID() string {
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
func ValidID(s string) bool {
	if len(s) != 43 {
		return false
	}
	b, e := base64.RawURLEncoding.DecodeString(s)
	return e == nil && len(b) == 32 && base64.RawURLEncoding.EncodeToString(b) == s
}
func Consumer(s string) bool { return s == "ai.openai" || s == "ai.nvidia" }
func validValue(s string) bool {
	if s == "" || len(s) > 4096 {
		return false
	}
	for _, b := range []byte(s) {
		if b < 33 || b > 126 {
			return false
		}
	}
	return true
}
func validText(s string, n int) bool {
	return s != "" && len(s) <= n && !strings.ContainsFunc(s, unicode.IsControl)
}
func audit(d *document, owner, operation, secret, enrollment string, ok bool, now time.Time) {
	// Refused caller-supplied identifiers must not turn the audit into a value echo.
	known := false
	for _, entry := range d.Secrets {
		if entry.ID == secret && entry.Owner == owner {
			known = true
		}
	}
	if !ValidID(secret) || (!ok && !known) {
		secret = ""
	}
	if !validText(enrollment, 64) {
		enrollment = ""
	}
	a := struct {
		Audit
		Owner string `json:"owner"`
	}{Audit{At: now.UTC(), Operation: operation, SecretID: secret, EnrollmentID: enrollment, Allowed: ok}, owner}
	d.Audit = append(d.Audit, a)
	if len(d.Audit) > 512 {
		d.Audit = d.Audit[len(d.Audit)-512:]
	}
}
func (v *Vault) paths() (string, string) {
	return filepath.Join(v.dir, "key"), filepath.Join(v.dir, "vault")
}
func (v *Vault) load() (document, []byte, error) {
	var d document
	if checkPrivateDir(v.dir) != nil {
		return d, nil, ErrUnavailable
	}
	kp, dp := v.paths()
	key, e := readPrivate(kp, 32)
	if e != nil || len(key) != 32 {
		return d, nil, ErrUnavailable
	}
	data, e := readPrivate(dp, maxDocument)
	if e != nil {
		return d, nil, ErrUnavailable
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return d, nil, ErrUnavailable
	}
	g, e := cipher.NewGCM(block)
	if e != nil || len(data) < g.NonceSize() {
		return d, nil, ErrUnavailable
	}
	plain, e := g.Open(nil, data[:g.NonceSize()], data[g.NonceSize():], []byte(associatedData))
	if e != nil {
		return d, nil, ErrUnavailable
	}
	dec := json.NewDecoder(bytes.NewReader(plain))
	dec.DisallowUnknownFields()
	if dec.Decode(&d) != nil || dec.Decode(new(any)) != io.EOF || d.Version != 1 || d.Owners == nil || len(d.Owners) > 64 || len(d.Secrets) > 64 || len(d.Grants) > 256 || len(d.Audit) > 512 {
		return document{}, nil, ErrUnavailable
	}
	ids := map[string]bool{}
	for _, s := range d.Secrets {
		if !ValidID(s.ID) || ids[s.ID] || !validText(s.Owner, 256) || !validText(s.Name, 80) || !Consumer(s.Consumer) || s.Version == 0 || !validValue(s.Value) {
			return document{}, nil, ErrUnavailable
		}
		ids[s.ID] = true
	}
	gids := map[string]bool{}
	for _, g := range d.Grants {
		if !ValidID(g.ID) || gids[g.ID] || !ids[g.SecretID] || !validText(g.Owner, 256) || !validText(g.EnrollmentID, 64) || !validText(g.Fingerprint, 128) || !Consumer(g.Consumer) || !validText(g.SourceNode, 64) || !validText(g.SourceOwner, 256) {
			return document{}, nil, ErrUnavailable
		}
		gids[g.ID] = true
	}
	return d, key, nil
}
func (v *Vault) save(d document, key []byte) error {
	plain, e := json.Marshal(d)
	if e != nil || len(plain) > maxDocument-64 {
		return ErrUnavailable
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return ErrUnavailable
	}
	g, e := cipher.NewGCM(block)
	if e != nil {
		return ErrUnavailable
	}
	nonce := make([]byte, g.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return ErrUnavailable
	}
	data := g.Seal(nonce, nonce, plain, []byte(associatedData))
	_, dp := v.paths()
	f, e := os.CreateTemp(v.dir, ".vault-*")
	if e != nil {
		return ErrUnavailable
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(data); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil || ce != nil {
		return ErrUnavailable
	}
	if e = os.Rename(f.Name(), dp); e != nil {
		return ErrUnavailable
	}
	if syncPrivateDir(v.dir) != nil {
		return ErrUnavailable
	}
	return nil
}

// Setup is an explicit operator action. Reads/startup do not create a vault.
func (v *Vault) Setup(owner string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !validText(owner, 256) {
		return ErrDenied
	}
	if e := makePrivateDir(v.dir); e != nil {
		return ErrUnavailable
	}
	kp, dp := v.paths()
	_, de := os.Lstat(dp)
	key, ke := readPrivate(kp, 32)
	if errors.Is(de, os.ErrNotExist) && errors.Is(ke, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, e := rand.Read(key); e != nil {
			return ErrUnavailable
		}
		if e := createPrivate(kp, key); e != nil {
			return ErrUnavailable
		}
		ke = nil
	}
	if ke != nil || len(key) != 32 {
		return ErrUnavailable
	}
	var d document
	if errors.Is(de, os.ErrNotExist) {
		d = document{Version: 1, Owners: map[string]bool{}}
	} else {
		var e error
		d, key, e = v.load()
		if e != nil {
			return e
		}
	}
	if len(d.Owners) >= 64 && !d.Owners[owner] {
		return ErrLimit
	}
	d.Owners[owner] = true
	audit(&d, owner, "enable", "", "", true, v.now())
	return v.save(d, key)
}
func (v *Vault) View(owner string) (View, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := View{Secrets: []Secret{}, Grants: []Grant{}, Audit: []Audit{}}
	if _, e := os.Lstat(v.dir); errors.Is(e, os.ErrNotExist) {
		return out, nil
	}
	d, _, e := v.load()
	if e != nil {
		return out, e
	}
	out.Enabled = d.Owners[owner]
	for _, s := range d.Secrets {
		if s.Owner == owner {
			out.Secrets = append(out.Secrets, s.Secret)
		}
	}
	for _, g := range d.Grants {
		if g.Owner == owner {
			info := g.Grant
			info.EnrollmentFingerprint = g.Fingerprint
			for _, secret := range d.Secrets {
				if secret.ID == g.SecretID && secret.Owner == owner && secret.Enabled && secret.Consumer == g.Consumer {
					info.Active = true
				}
			}
			out.Grants = append(out.Grants, info)
		}
	}
	for _, a := range d.Audit {
		if a.Owner == owner {
			out.Audit = append(out.Audit, a.Audit)
		}
	}
	return out, nil
}
func (v *Vault) Put(owner, id, name, value, consumer string, enabled bool) (Secret, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	d, key, e := v.load()
	if e != nil {
		return Secret{}, e
	}
	if !d.Owners[owner] || !validText(name, 80) || !Consumer(consumer) || (value != "" && !validValue(value)) {
		return Secret{}, ErrDenied
	}
	index := -1
	count := 0
	for i, s := range d.Secrets {
		if s.Owner == owner {
			count++
		}
		if s.ID == id {
			index = i
		}
	}
	if index < 0 {
		if id != "" || value == "" {
			return Secret{}, ErrDenied
		}
		if len(d.Secrets) >= 64 || count >= 32 {
			return Secret{}, ErrLimit
		}
		id = randomID()
		d.Secrets = append(d.Secrets, storedSecret{Secret: Secret{ID: id, Owner: owner, Version: 0}})
		index = len(d.Secrets) - 1
	}
	s := &d.Secrets[index]
	if s.Owner != owner || (s.Version != 0 && s.Consumer != consumer) || s.Version == ^uint64(0) {
		return Secret{}, ErrDenied
	}
	if value != "" {
		s.Value = value
	}
	s.Name = name
	s.Consumer = consumer
	s.Enabled = enabled
	s.Version++
	audit(&d, owner, "store", id, "", true, v.now())
	if e = v.save(d, key); e != nil {
		return Secret{}, e
	}
	return s.Secret, nil
}
func (v *Vault) Grant(scope Scope) (Grant, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	d, key, e := v.load()
	if e != nil {
		return Grant{}, e
	}
	if !validScope(scope) || !d.Owners[scope.Owner] {
		return Grant{}, ErrDenied
	}
	found := false
	for _, s := range d.Secrets {
		if s.ID == scope.SecretID && s.Owner == scope.Owner && s.Enabled && s.Consumer == scope.Consumer {
			found = true
		}
	}
	if !found {
		return Grant{}, ErrDenied
	}
	for _, g := range d.Grants {
		if matchGrant(g, scope) {
			return g.Grant, nil
		}
	}
	if len(d.Grants) >= 256 {
		return Grant{}, ErrLimit
	}
	g := storedGrant{Grant: Grant{ID: randomID(), SecretID: scope.SecretID, EnrollmentID: scope.EnrollmentID, SourceNode: scope.SourceNode, SourceOwner: scope.SourceOwner, Consumer: scope.Consumer}, Owner: scope.Owner, Fingerprint: scope.EnrollmentFingerprint}
	d.Grants = append(d.Grants, g)
	audit(&d, scope.Owner, "grant", scope.SecretID, scope.EnrollmentID, true, v.now())
	if e = v.save(d, key); e != nil {
		return Grant{}, e
	}
	return g.Grant, nil
}
func (v *Vault) Revoke(owner, id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	d, key, e := v.load()
	if e != nil {
		return e
	}
	for i, g := range d.Grants {
		if g.ID == id && g.Owner == owner {
			d.Grants = append(d.Grants[:i], d.Grants[i+1:]...)
			audit(&d, owner, "revoke", g.SecretID, g.EnrollmentID, true, v.now())
			return v.save(d, key)
		}
	}
	return ErrDenied
}
func validScope(s Scope) bool {
	return validText(s.Owner, 256) && ValidID(s.SecretID) && validText(s.EnrollmentID, 64) && validText(s.EnrollmentFingerprint, 128) && validText(s.SourceNode, 64) && validText(s.SourceOwner, 256) && Consumer(s.Consumer) && s.Project == ""
}
func matchGrant(g storedGrant, s Scope) bool {
	return g.Owner == s.Owner && g.SecretID == s.SecretID && g.EnrollmentID == s.EnrollmentID && g.Fingerprint == s.EnrollmentFingerprint && g.SourceNode == s.SourceNode && g.SourceOwner == s.SourceOwner && g.Consumer == s.Consumer
}
func authorized(d document, s Scope) (storedSecret, storedGrant, bool) {
	if !validScope(s) || !d.Owners[s.Owner] {
		return storedSecret{}, storedGrant{}, false
	}
	for _, g := range d.Grants {
		if matchGrant(g, s) {
			for _, secret := range d.Secrets {
				if secret.ID == s.SecretID && secret.Owner == s.Owner && secret.Enabled && secret.Consumer == s.Consumer {
					return secret, g, true
				}
			}
		}
	}
	return storedSecret{}, storedGrant{}, false
}
func (v *Vault) Check(s Scope) (Secret, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	d, key, e := v.load()
	if e != nil {
		return Secret{}, e
	}
	secret, _, ok := authorized(d, s)
	audit(&d, s.Owner, "availability", s.SecretID, s.EnrollmentID, ok, v.now())
	if e = v.save(d, key); e != nil {
		return Secret{}, e
	}
	if !ok {
		return Secret{}, ErrDenied
	}
	return secret.Secret, nil
}
func (v *Vault) Issue(s Scope) (string, time.Time, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	d, key, e := v.load()
	if e != nil {
		return "", time.Time{}, e
	}
	secret, g, ok := authorized(d, s)
	now := v.now()
	audit(&d, s.Owner, "issue", s.SecretID, s.EnrollmentID, ok, now)
	if !ok {
		if e = v.save(d, key); e != nil {
			return "", time.Time{}, e
		}
		return "", time.Time{}, ErrDenied
	}
	for k, h := range v.handles {
		if !now.Before(h.Expires) {
			delete(v.handles, k)
		}
	}
	if len(v.handles) >= 256 {
		return "", time.Time{}, ErrLimit
	}
	token := randomID()
	expires := now.Add(handleTTL)
	if e = v.save(d, key); e != nil {
		return "", time.Time{}, e
	}
	v.handles[sha256.Sum256([]byte(token))] = handle{s, g.ID, secret.Version, expires}
	return token, expires, nil
}
func (v *Vault) Redeem(s Scope, token string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	d, key, e := v.load()
	if e != nil {
		return "", e
	}
	h, exists := v.handles[sha256.Sum256([]byte(token))]
	secret, g, ok := authorized(d, s)
	ok = ok && ValidID(token) && exists && h.Scope == s && v.now().Before(h.Expires) && h.GrantID == g.ID && h.Version == secret.Version
	if exists && h.Scope == s {
		delete(v.handles, sha256.Sum256([]byte(token)))
	}
	audit(&d, s.Owner, "redeem", s.SecretID, s.EnrollmentID, ok, v.now())
	if e = v.save(d, key); e != nil {
		return "", e
	}
	if !ok {
		return "", ErrDenied
	}
	return secret.Value, nil
}

// Fingerprint is safe to persist as an enrollment fence, never a bearer.
func Fingerprint(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (v *Vault) Delete(owner, id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	d, key, e := v.load()
	if e != nil {
		return e
	}
	found := false
	for i, s := range d.Secrets {
		if s.ID == id && s.Owner == owner {
			d.Secrets = append(d.Secrets[:i], d.Secrets[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return ErrDenied
	}
	grants := d.Grants[:0]
	for _, g := range d.Grants {
		if g.SecretID != id {
			grants = append(grants, g)
		}
	}
	d.Grants = grants
	audit(&d, owner, "delete", id, "", true, v.now())
	return v.save(d, key)
}

// Available returns only metadata for the exact enrollment/account/consumer.
func (v *Vault) Available(scope Scope) ([]Secret, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	d, key, e := v.load()
	if e != nil {
		return nil, e
	}
	out := []Secret{}
	if scope.SecretID != "" || scope.Project != "" || !Consumer(scope.Consumer) || !d.Owners[scope.Owner] {
		return nil, ErrDenied
	}
	seen := map[string]bool{}
	for _, g := range d.Grants {
		candidate := scope
		candidate.SecretID = g.SecretID
		if secret, _, ok := authorized(d, candidate); ok && !seen[secret.ID] {
			out = append(out, secret.Secret)
			seen[secret.ID] = true
		}
	}
	audit(&d, scope.Owner, "list", "", scope.EnrollmentID, true, v.now())
	if e = v.save(d, key); e != nil {
		return nil, e
	}
	return out, nil
}
