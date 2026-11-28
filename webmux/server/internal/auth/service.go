package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

const TokenTTL = 8 * time.Hour
const TicketTTL = time.Minute

type ticket struct {
	generation string
	username   string
	expires    time.Time
}

type sessionClaims struct {
	jwt.RegisteredClaims
	Generation string `json:"account_generation,omitempty"`
}

// AccountGeneration identifies a salted account incarnation without exposing its hash.
func AccountGeneration(username, passwordHash string) string {
	b, _ := json.Marshal(struct{ Username, Generation string }{username, passwordHash})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

type Service struct {
	store   *storage.Store
	secret  []byte
	mu      sync.Mutex
	tickets map[string]ticket
	now     func() time.Time
}

func New(store *storage.Store, secretOverride string) (*Service, error) {
	config, err := LoadConfig(store)
	if err != nil {
		return nil, err
	}
	secret := secretOverride
	if secret == "" {
		secret = config.Auth.JWTSecret
	}
	if secret == "" {
		generated, err := randomHex(32)
		if err != nil {
			return nil, err
		}
		err = storage.UpdateConfig(store, "auth.yaml", func(c *Config) error {
			if err := c.Validate(); err != nil {
				return err
			}
			if c.Auth.JWTSecret == "" {
				c.Auth.JWTSecret = generated
			}
			secret = c.Auth.JWTSecret
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return &Service{store: store, secret: []byte(secret), tickets: make(map[string]ticket), now: time.Now}, nil
}

func (s *Service) Sign(username string) (string, error) {
	c, e := LoadConfig(s.store)
	if e != nil {
		return "", e
	}
	generation := ""
	if c.Auth.Mode == "local" {
		for _, u := range c.Auth.Users {
			if u.Username == username {
				generation = AccountGeneration(username, u.PasswordHash)
			}
		}
	}
	return s.SignGeneration(username, generation)
}

// SignGeneration binds the exact password record authenticated by the caller.
func (s *Service) SignGeneration(username, generation string) (string, error) {
	now := s.now()
	claims := sessionClaims{RegisteredClaims: jwt.RegisteredClaims{Subject: username, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(TokenTTL))}, Generation: generation}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}
func (s *Service) claims(token string) (sessionClaims, error) {
	var claims sessionClaims
	_, e := jwt.ParseWithClaims(token, &claims, func(_ *jwt.Token) (any, error) { return s.secret, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithTimeFunc(s.now))
	if e != nil {
		return claims, e
	}
	if claims.Subject == "" {
		return claims, errors.New("token has no subject")
	}
	return claims, nil
}
func (s *Service) Verify(token string) (string, error) {
	claims, e := s.claims(token)
	if e != nil {
		return "", e
	}
	c, e := LoadConfig(s.store)
	if e != nil {
		return "", e
	}
	if c.Auth.Mode == "local" {
		current := ""
		for _, u := range c.Auth.Users {
			if u.Username == claims.Subject {
				current = AccountGeneration(u.Username, u.PasswordHash)
			}
		}
		if current == "" || claims.Generation != current {
			return "", errors.New("account session changed; sign in again")
		}
	}
	return claims.Subject, nil
}

// Generation is for sensitive operations. Legacy sessions must authenticate again.
func (s *Service) Generation(token string) (string, error) {
	claims, e := s.claims(token)
	if e != nil {
		return "", e
	}
	if len(claims.Generation) != 64 {
		return "", errors.New("Sign in again to use shared credentials")
	}
	return claims.Generation, nil
}

// Refresh preserves the authenticated generation; it can never upgrade a legacy
// or stale token to the current account incarnation without password authentication.
func (s *Service) Refresh(token string) (string, error) {
	if _, e := s.Verify(token); e != nil {
		return "", e
	}
	claims, e := s.claims(token)
	if e != nil {
		return "", e
	}
	return s.SignGeneration(claims.Subject, claims.Generation)
}

func (s *Service) currentGeneration(username string) (string, error) {
	c, e := LoadConfig(s.store)
	if e != nil {
		return "", e
	}
	if c.Auth.Mode == "none" {
		return "", nil
	}
	for _, u := range c.Auth.Users {
		if u.Username == username {
			return AccountGeneration(username, u.PasswordHash), nil
		}
	}
	return "", errors.New("account session changed; sign in again")
}
func (s *Service) IssueTicket(username string, expected ...string) (string, error) {
	generation, e := s.currentGeneration(username)
	if e != nil {
		return "", e
	}
	if len(expected) > 0 && expected[0] != generation {
		return "", errors.New("account session changed; sign in again")
	}
	id, err := randomHex(24)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for key, entry := range s.tickets {
		if !now.Before(entry.expires) {
			delete(s.tickets, key)
		}
	}
	s.tickets[id] = ticket{generation: generation, username: username, expires: now.Add(TicketTTL)}
	return id, nil
}

// ConsumeTicket is atomic: simultaneous upgrades cannot reuse a ticket.
func (s *Service) ConsumeTicket(id string) (string, bool) {
	s.mu.Lock()
	entry, ok := s.tickets[id]
	delete(s.tickets, id)
	valid := ok && s.now().Before(entry.expires)
	s.mu.Unlock()
	if !valid {
		return "", false
	}
	current, e := s.currentGeneration(entry.username)
	if e != nil || current != entry.generation {
		return "", false
	}
	return entry.username, true
}

func randomHex(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
