package auth

import (
	"errors"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

type User struct {
	Username     string         `yaml:"username"`
	PasswordHash string         `yaml:"password_hash"`
	Admin        bool           `yaml:"admin,omitempty"`
	Extra        map[string]any `yaml:",inline"`
}

type Config struct {
	Auth struct {
		Mode      string         `yaml:"mode"`
		Users     []User         `yaml:"users"`
		JWTSecret string         `yaml:"jwt_secret,omitempty"`
		Extra     map[string]any `yaml:",inline"`
	} `yaml:"auth"`
	Extra map[string]any `yaml:",inline"`
}

func LoadConfig(store *storage.Store) (Config, error) {
	var config Config
	if err := store.ReadConfig("auth.yaml", &config); err != nil {
		return config, err
	}
	return config, config.Validate()
}

func (c *Config) Validate() error {
	if c.Auth.Mode != "local" && c.Auth.Mode != "none" {
		return errors.New("auth.mode must be local or none")
	}
	seen := make(map[string]bool)
	for _, user := range c.Auth.Users {
		if user.Username == "" || user.PasswordHash == "" || seen[user.Username] {
			return errors.New("invalid or duplicate auth user")
		}
		seen[user.Username] = true
	}
	return nil
}

// IsAdmin preserves the legacy rule: if no account has admin: true, the
// first account owns the installation (including old admin: false entries).
func IsAdmin(users []User, username string) bool {
	for _, user := range users {
		if user.Admin {
			for _, candidate := range users {
				if candidate.Username == username {
					return candidate.Admin
				}
			}
			return false
		}
	}
	return len(users) > 0 && users[0].Username == username
}

func (c *Config) HasUser(username string) bool {
	for _, user := range c.Auth.Users {
		if user.Username == username {
			return true
		}
	}
	return false
}
