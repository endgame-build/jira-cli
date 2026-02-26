// Package auth provides credential storage and resolution for jira-cli.
// Tokens are stored in the OS keyring when available, with automatic
// fallback to an encrypted-at-rest JSON file when keyring is unavailable.
package auth

import (
	"fmt"
	"io"

	"github.com/zalando/go-keyring"
)

const serviceName = "jira-cli"

// ErrTokenNotFound is returned when a token is not found in any store.
var ErrTokenNotFound = keyring.ErrNotFound

// tokenKey builds the keyring key for a given profile: "{profile}-token".
func tokenKey(profile string) string {
	return profile + "-token"
}

// TokenStore abstracts credential storage for API tokens.
type TokenStore interface {
	StoreToken(profile, token string) error
	RetrieveToken(profile string) (string, error)
	DeleteToken(profile string) error
}

// keyringStore stores tokens in the OS keyring.
type keyringStore struct{}

func (k *keyringStore) StoreToken(profile, token string) error {
	return keyring.Set(serviceName, tokenKey(profile), token)
}

func (k *keyringStore) RetrieveToken(profile string) (string, error) {
	return keyring.Get(serviceName, tokenKey(profile))
}

func (k *keyringStore) DeleteToken(profile string) error {
	return keyring.Delete(serviceName, tokenKey(profile))
}

// fallbackStore wraps a primary store (keyring) with a plaintext file
// fallback. When the primary fails, it transparently uses the fallback
// and prints a warning to stderr.
type fallbackStore struct {
	primary  TokenStore
	fallback *fileTokenStore
	stderr   io.Writer
}

// NewTokenStore returns a TokenStore that tries the OS keyring first and
// falls back to tokens.json when the keyring is unavailable.
// The tokensPath argument is the path to the fallback tokens.json file.
// stderr is used to print warnings when keyring is unavailable.
func NewTokenStore(tokensPath string, stderr io.Writer) TokenStore {
	return &fallbackStore{
		primary:  &keyringStore{},
		fallback: &fileTokenStore{path: tokensPath},
		stderr:   stderr,
	}
}

func (s *fallbackStore) StoreToken(profile, token string) error {
	err := s.primary.StoreToken(profile, token)
	if err == nil {
		return nil
	}
	fmt.Fprintf(s.stderr, "warning: keyring unavailable (%v), using plaintext token storage\n", err)
	return s.fallback.StoreToken(profile, token)
}

func (s *fallbackStore) RetrieveToken(profile string) (string, error) {
	token, err := s.primary.RetrieveToken(profile)
	if err == nil {
		return token, nil
	}
	// If keyring says "not found", don't fall back — the token genuinely doesn't exist there.
	if err == keyring.ErrNotFound {
		// Still try fallback in case it was stored there previously.
		return s.fallback.RetrieveToken(profile)
	}
	// Keyring is broken/unavailable — warn and fall back (symmetric with StoreToken).
	fmt.Fprintf(s.stderr, "warning: keyring unavailable (%v), trying plaintext token storage\n", err)
	return s.fallback.RetrieveToken(profile)
}

func (s *fallbackStore) DeleteToken(profile string) error {
	// Try deleting from both stores. Keyring errors (other than not-found) are ignored.
	keyErr := s.primary.DeleteToken(profile)
	fileErr := s.fallback.DeleteToken(profile)

	// If both say not-found, return not-found.
	if keyErr == keyring.ErrNotFound && fileErr == keyring.ErrNotFound {
		return keyring.ErrNotFound
	}
	// If either succeeded, consider it a success.
	if keyErr == nil || fileErr == nil {
		return nil
	}
	// If file store had a real error, return it.
	if fileErr != nil && fileErr != keyring.ErrNotFound {
		return fileErr
	}
	return nil
}
