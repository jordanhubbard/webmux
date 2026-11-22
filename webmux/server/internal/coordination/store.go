package coordination

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var legacyBuckets = []string{"meta", "members", "claims", "messages", "inboxes", "outboxes", "receipts", "reservations", "fences"}
var buckets = append(append([]string{}, legacyBuckets...), "remote_outbox", "remote_inbox", "remote_active")

// Service has one owner per WEBMUX_HOME, like the session broker. All persistent
// transitions use one serializable transaction; no network/provider work occurs
// inside it. The database lock also refuses simultaneous processes using a home.
type Service struct {
	home   string
	mu     sync.Mutex
	db     *bolt.DB
	closed bool
	Now    func() time.Time
}

func New(home string) *Service { return &Service{home: home, Now: time.Now} }
func (s *Service) database() (*bolt.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	if s.db != nil {
		return s.db, nil
	}
	directory := filepath.Join(s.home, "data", "coordination")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	db, err := bolt.Open(filepath.Join(directory, "state.db"), 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	s.db = db
	return db, nil
}
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
func bucket(tx *bolt.Tx, name string) *bolt.Bucket { return tx.Bucket([]byte(name)) }
func get[T any](b *bolt.Bucket, key string) (T, bool, error) {
	var result T
	value := b.Get([]byte(key))
	if value == nil {
		return result, false, nil
	}
	err := json.Unmarshal(value, &result)
	return result, true, err
}
func put(b *bolt.Bucket, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return b.Put([]byte(key), data)
}
func readAll[T any](b *bolt.Bucket) ([]T, error) {
	out := []T{}
	err := b.ForEach(func(_, v []byte) error {
		if v == nil {
			return errors.New("unexpected nested record")
		}
		var entry T
		if err := json.Unmarshal(v, &entry); err != nil {
			return err
		}
		out = append(out, entry)
		return nil
	})
	return out, err
}
func initialize(tx *bolt.Tx, node string, now time.Time) error {
	if !validHex(node, 16) {
		return ErrInvalid
	}
	if meta := tx.Bucket([]byte("meta")); meta != nil {
		if string(meta.Get([]byte("node"))) != node {
			return ErrIdentity
		}
		if (string(meta.Get([]byte("version"))) != "1" && string(meta.Get([]byte("version"))) != "2") || len(meta.Get([]byte("key"))) != 32 || (string(meta.Get([]byte("enabled"))) != "1" && string(meta.Get([]byte("enabled"))) != "0") {
			return errors.New("invalid coordination database metadata")
		}
		required := buckets
		if string(meta.Get([]byte("version"))) == "1" {
			required = legacyBuckets
		}
		for _, name := range required {
			if bucket(tx, name) == nil {
				return errors.New("missing coordination database bucket")
			}
		}
		clock, exists, err := get[time.Time](meta, "clock")
		if err != nil || !exists || clock.IsZero() {
			return errors.New("invalid coordination authority clock")
		}
		if string(meta.Get([]byte("version"))) == "1" {
			for _, name := range []string{"remote_outbox", "remote_inbox", "remote_active"} {
				if _, err := tx.CreateBucket([]byte(name)); err != nil {
					return err
				}
			}
			epoch := make([]byte, 16)
			if _, err := rand.Read(epoch); err != nil {
				return err
			}
			if err := meta.Put([]byte("history_epoch"), []byte(hex.EncodeToString(epoch))); err != nil {
				return err
			}
			if err := meta.Put([]byte("version"), []byte("2")); err != nil {
				return err
			}
		}
		if !validHex(string(meta.Get([]byte("history_epoch"))), 16) {
			return errors.New("invalid coordination history epoch")
		}
		return nil
	}
	for _, name := range buckets {
		if _, err := tx.CreateBucket([]byte(name)); err != nil {
			return err
		}
	}
	meta := bucket(tx, "meta")
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	for name, value := range map[string][]byte{"node": []byte(node), "version": []byte("2"), "key": key, "enabled": []byte("1")} {
		if err := meta.Put([]byte(name), value); err != nil {
			return err
		}
	}
	epoch := make([]byte, 16)
	if _, err := rand.Read(epoch); err != nil {
		return err
	}
	if err := meta.Put([]byte("history_epoch"), []byte(hex.EncodeToString(epoch))); err != nil {
		return err
	}
	return put(meta, "clock", now)
}
func enabled(tx *bolt.Tx) bool { return string(bucket(tx, "meta").Get([]byte("enabled"))) == "1" }
func authorityTime(tx *bolt.Tx, now time.Time) (time.Time, error) {
	previous, exists, err := get[time.Time](bucket(tx, "meta"), "clock")
	if err != nil {
		return now, err
	}
	now = now.UTC()
	if exists && now.Before(previous) {
		return previous, ErrClock
	}
	return now, put(bucket(tx, "meta"), "clock", now)
}
func sign(key []byte, data string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("webmux-session-coordination/v1\n" + data))
	return hex.EncodeToString(mac.Sum(nil))
}
func capability(tx *bolt.Tx, ref Ref) string {
	data, _ := json.Marshal(ref)
	payload := base64.RawURLEncoding.EncodeToString(data)
	return "wmc1." + payload + "." + sign(bucket(tx, "meta").Get([]byte("key")), payload)
}
func authenticate(tx *bolt.Tx, token string) (Member, error) {
	var member Member
	if len(token) > 2048 {
		return member, ErrUnauthorized
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "wmc1" || !hmac.Equal([]byte(parts[2]), []byte(sign(bucket(tx, "meta").Get([]byte("key")), parts[1]))) {
		return member, ErrUnauthorized
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return member, ErrUnauthorized
	}
	var ref Ref
	if json.Unmarshal(raw, &ref) != nil || !validRef(ref) || ref.Node != string(bucket(tx, "meta").Get([]byte("node"))) {
		return member, ErrUnauthorized
	}
	member, exists, err := get[Member](bucket(tx, "members"), ref.Key())
	if err != nil {
		return member, err
	}
	if !exists || member.Ref != ref || !member.Present || !member.Active || member.Revoked != nil {
		return member, ErrUnauthorized
	}
	return member, nil
}
func leaseDuration(seconds int) (time.Duration, bool) {
	if seconds == 0 {
		seconds = DefaultLease
	}
	return time.Duration(seconds) * time.Second, seconds >= 15 && seconds <= 900
}
func sequence(n uint64) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, n)
	return key
}

func (s *Service) Subject(node, token string) (Ref, error) {
	db, err := s.database()
	if err != nil {
		return Ref{}, err
	}
	var ref Ref
	err = db.View(func(tx *bolt.Tx) error {
		meta := bucket(tx, "meta")
		if meta == nil || string(meta.Get([]byte("node"))) != node {
			return ErrUnauthorized
		}
		if !enabled(tx) {
			return ErrDisabled
		}
		member, err := authenticate(tx, token)
		if err != nil {
			return err
		}
		ref = member.Ref
		return nil
	})
	return ref, err
}
func (s *Service) MarkProvisioned(node string, ref Ref) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	return db.Update(func(tx *bolt.Tx) error {
		if err := initialize(tx, node, s.Now()); err != nil {
			return err
		}
		member, exists, err := get[Member](bucket(tx, "members"), ref.Key())
		if err != nil {
			return err
		}
		if !exists || member.Revoked != nil {
			return ErrUnauthorized
		}
		if member.Awareness == "interface-unprovisioned" {
			member.Awareness = "interface-provisioned"
			return put(bucket(tx, "members"), ref.Key(), member)
		}
		return nil
	})
}
