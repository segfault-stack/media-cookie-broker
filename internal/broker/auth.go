package broker

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/crypto/argon2"
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{0,63}$`)

type Auth struct{ store *Store }

func NewAuth(store *Store) *Auth { return &Auth{store: store} }

func (a *Auth) Authenticate(ctx context.Context, username, password, role, provider, profile string) bool {
	encoded, err := a.store.userPasswordHash(ctx, username, role, provider, profile)
	return err == nil && VerifyPassword(encoded, password)
}

func (a *Auth) AuthenticateRole(ctx context.Context, username, password, role string) bool {
	encoded, err := a.store.userRolePasswordHash(ctx, username, role)
	return err == nil && VerifyPassword(encoded, password)
}

func ValidUsername(username string) bool { return usernamePattern.MatchString(username) }

func HashPassword(password string) (string, error) {
	if len(password) < 20 {
		return "", errors.New("password must contain at least 20 characters")
	}
	salt := make([]byte, 16)
	if _, err := cryptoRead(salt); err != nil {
		return "", err
	}
	const memory, iterations, parallelism, keyLen = uint32(64 * 1024), uint32(3), uint8(2), uint32(32)
	key := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, iterations, parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func VerifyPassword(encoded, password string) bool {
	memory, iterations, parallelism, salt, expected, err := decodeArgon2(encoded)
	if err != nil {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func decodeArgon2(encoded string) (uint32, uint32, uint8, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return 0, 0, 0, nil, nil, errors.New("unsupported argon2id encoding")
	}
	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return 0, 0, 0, nil, nil, err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return 0, 0, 0, nil, nil, err
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return 0, 0, 0, nil, nil, err
	}
	if memory < 8*1024 || memory > 256*1024 || iterations == 0 || iterations > 10 ||
		parallelism == 0 || parallelism > 8 || len(salt) < 16 || len(salt) > 64 || len(key) < 16 || len(key) > 64 {
		return 0, 0, 0, nil, nil, errors.New("unsafe argon2id parameters")
	}
	return memory, iterations, parallelism, salt, key, nil
}
