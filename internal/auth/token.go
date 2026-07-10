package auth

import (
	"crypto/sha256"
	"encoding/hex"
)

const (
	tokenPrefixLen = 12
	apiTokenRand   = 32
)

type GeneratedToken struct {
	Full   string
	Prefix string
	Hash   string
}

func GenerateAPIToken() (GeneratedToken, error) {
	raw, err := randomBase62(apiTokenRand)
	if err != nil {
		return GeneratedToken{}, err
	}
	full := "dodo_" + raw
	return GeneratedToken{
		Full:   full,
		Prefix: full[:tokenPrefixLen],
		Hash:   hashToken(full),
	}, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

const sessionRand = 48

type GeneratedSession struct {
	Full string
	Hash string
}

func GenerateSession() (GeneratedSession, error) {
	raw, err := randomBase62(sessionRand)
	if err != nil {
		return GeneratedSession{}, err
	}
	full := "dodo_" + raw
	return GeneratedSession{
		Full: full,
		Hash: hashToken(full),
	}, nil
}

func HashToken(token string) string { return hashToken(token) }
