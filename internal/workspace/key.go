package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
)

var (
	ErrInvalid = errors.New("workspace key is malformed")
	keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43,128}$`)
)

func Hash(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !keyPattern.MatchString(value) {
		return "", ErrInvalid
	}
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:]), nil
}
