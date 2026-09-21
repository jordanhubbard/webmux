package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

const TokenTTL = 8 * time.Hour
const TicketTTL = time.Minute

type ticket struct {
	username string
	expires  time.Time
}

type Service struct {
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
	return &Service{secret: []byte(secret), tickets: make(map[string]ticket), now: time.Now}, nil
}

func (s *Service) Sign(username string) (string, error) {
	now := s.now()
	claims := jwt.RegisteredClaims{Subject: username, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(TokenTTL))}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func (s *Service) Verify(token string) (string, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(token, &claims, func(_ *jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithTimeFunc(s.now))
	if err != nil {
		return "", err
	}
	if claims.Subject == "" {
		return "", errors.New("token has no subject")
	}
	return claims.Subject, nil
}

func (s *Service) IssueTicket(username string) (string, error) {
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
	s.tickets[id] = ticket{username, now.Add(TicketTTL)}
	return id, nil
}

// ConsumeTicket is atomic: simultaneous upgrades cannot reuse a ticket.
func (s *Service) ConsumeTicket(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.tickets[id]
	delete(s.tickets, id)
	if !ok || !s.now().Before(entry.expires) {
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
