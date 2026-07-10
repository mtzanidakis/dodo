package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"math/big"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory      = 64 * 1024
	argonIterations  = 3
	argonParallelism = 2
	argonKeyLength   = 32
)

func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("empty password")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLength)
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)
	return "argon2id$" + b64Salt + "$" + b64Hash, nil
}

func VerifyPassword(password, encoded string) bool {
	if encoded == "" || password == "" {
		return false
	}
	saltStr, hashStr, ok := splitEncoded(encoded)
	if !ok {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(saltStr)
	if err != nil {
		return false
	}
	hash, err := base64.RawStdEncoding.DecodeString(hashStr)
	if err != nil {
		return false
	}
	other := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLength)
	if len(other) != len(hash) {
		return false
	}
	return subtle.ConstantTimeCompare(other, hash) == 1
}

func splitEncoded(s string) (string, string, bool) {
	want := "argon2id$"
	if len(s) <= len(want) || s[:len(want)] != want {
		return "", "", false
	}
	rest := s[len(want):]
	for i := 0; i < len(rest); i++ {
		if rest[i] == '$' {
			return rest[:i], rest[i+1:], true
		}
	}
	return "", "", false
}

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func randomBase62(n int) (string, error) {
	b := make([]byte, n)
	max := big.NewInt(int64(len(base62Chars)))
	for i := range b {
		r, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = base62Chars[r.Int64()]
	}
	return string(b), nil
}
