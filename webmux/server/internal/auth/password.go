package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Match node-argon2's existing Argon2id defaults and PHC storage format.
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 65536, 4, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=4$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func VerifyPassword(encoded, password string) (bool, error) {
	invalid := errors.New("invalid or unsupported Argon2id password hash")
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, invalid
	}
	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return false, invalid
	}
	var values [3]uint64
	for i, prefix := range []string{"m=", "t=", "p="} {
		if !strings.HasPrefix(params[i], prefix) {
			return false, invalid
		}
		n, err := strconv.ParseUint(strings.TrimPrefix(params[i], prefix), 10, 32)
		if err != nil {
			return false, invalid
		}
		values[i] = n
	}
	memory, iterations, lanes := values[0], values[1], values[2]
	// Bound resource use even if an operator accidentally imports a corrupted
	// hash. Covers existing node-argon2 defaults; unsupported costs fail closed.
	if lanes < 1 || lanes > 16 || iterations < 1 || iterations > 10 || memory < 8*lanes || memory > 256*1024 {
		return false, invalid
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return false, invalid
	}
	want, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(want) < 16 || len(want) > 64 {
		return false, invalid
	}
	got := argon2.IDKey([]byte(password), salt, uint32(iterations), uint32(memory), uint8(lanes), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
