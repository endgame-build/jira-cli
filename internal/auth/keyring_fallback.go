package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

// fileTokenStore stores tokens in a JSON file as a fallback when the OS keyring
// is unavailable. Token keys use the same "{profile}-token" format.
type fileTokenStore struct {
	path string
}

// tokensFile is the on-disk JSON representation: map of key → token.
type tokensFile map[string]string

func (f *fileTokenStore) StoreToken(profile, token string) error {
	tokens, err := f.load()
	if err != nil {
		return err
	}
	tokens[tokenKey(profile)] = token
	return f.save(tokens)
}

func (f *fileTokenStore) RetrieveToken(profile string) (string, error) {
	tokens, err := f.load()
	if err != nil {
		return "", err
	}
	tok, ok := tokens[tokenKey(profile)]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return tok, nil
}

func (f *fileTokenStore) DeleteToken(profile string) error {
	tokens, err := f.load()
	if err != nil {
		return err
	}
	key := tokenKey(profile)
	if _, ok := tokens[key]; !ok {
		return keyring.ErrNotFound
	}
	delete(tokens, key)
	return f.save(tokens)
}

// load reads the tokens file, returning an empty map if it doesn't exist.
func (f *fileTokenStore) load() (tokensFile, error) {
	raw, err := os.ReadFile(f.path)
	if os.IsNotExist(err) {
		return make(tokensFile), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading tokens file: %w", err)
	}

	var tokens tokensFile
	if err := json.Unmarshal(raw, &tokens); err != nil {
		return nil, fmt.Errorf("parsing tokens file: %w", err)
	}
	return tokens, nil
}

// save writes the tokens file using write-to-temp-then-rename for atomicity.
func (f *fileTokenStore) save(tokens tokensFile) error {
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating tokens dir: %w", err)
	}

	raw, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling tokens: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "tokens-*.json.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if err := os.Chmod(tmpPath, 0o600); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("setting file permissions: %w", err)
	}

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, f.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming tokens file: %w", err)
	}

	return nil
}
