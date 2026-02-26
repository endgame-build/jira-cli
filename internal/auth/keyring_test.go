package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestKeyringStore_StoreRetrieveDelete(t *testing.T) {
	keyring.MockInit()
	store := &keyringStore{}

	// Store
	if err := store.StoreToken("default", "my-token"); err != nil {
		t.Fatalf("StoreToken: %v", err)
	}

	// Retrieve
	tok, err := store.RetrieveToken("default")
	if err != nil {
		t.Fatalf("RetrieveToken: %v", err)
	}
	if tok != "my-token" {
		t.Errorf("got %q, want %q", tok, "my-token")
	}

	// Delete
	if err := store.DeleteToken("default"); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}

	// Retrieve after delete → ErrNotFound
	_, err = store.RetrieveToken("default")
	if err != keyring.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestKeyringStore_RetrieveNotFound(t *testing.T) {
	keyring.MockInit()
	store := &keyringStore{}

	_, err := store.RetrieveToken("nonexistent")
	if err != keyring.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestKeyringStore_DeleteNotFound(t *testing.T) {
	keyring.MockInit()
	store := &keyringStore{}

	err := store.DeleteToken("nonexistent")
	if err != keyring.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestKeyringStore_KeyFormat(t *testing.T) {
	keyring.MockInit()
	store := &keyringStore{}

	_ = store.StoreToken("work", "work-token")

	// Verify the key format is "{profile}-token" by direct keyring access
	tok, err := keyring.Get(serviceName, "work-token")
	if err != nil {
		t.Fatalf("keyring.Get: %v", err)
	}
	if tok != "work-token" {
		t.Errorf("keyring key format mismatch: got %q", tok)
	}
}

func TestFileTokenStore_StoreRetrieveDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	store := &fileTokenStore{path: path}

	// Store
	if err := store.StoreToken("default", "file-token"); err != nil {
		t.Fatalf("StoreToken: %v", err)
	}

	// File should exist
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("tokens.json not created: %v", err)
	}

	// Retrieve
	tok, err := store.RetrieveToken("default")
	if err != nil {
		t.Fatalf("RetrieveToken: %v", err)
	}
	if tok != "file-token" {
		t.Errorf("got %q, want %q", tok, "file-token")
	}

	// Delete
	if err := store.DeleteToken("default"); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}

	_, err = store.RetrieveToken("default")
	if err != keyring.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestFileTokenStore_RetrieveNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	store := &fileTokenStore{path: path}

	_, err := store.RetrieveToken("nonexistent")
	if err != keyring.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFileTokenStore_DeleteNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	store := &fileTokenStore{path: path}

	err := store.DeleteToken("nonexistent")
	if err != keyring.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFileTokenStore_MultipleProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	store := &fileTokenStore{path: path}

	_ = store.StoreToken("work", "work-tok")
	_ = store.StoreToken("personal", "personal-tok")

	tok1, _ := store.RetrieveToken("work")
	tok2, _ := store.RetrieveToken("personal")

	if tok1 != "work-tok" {
		t.Errorf("work: got %q, want %q", tok1, "work-tok")
	}
	if tok2 != "personal-tok" {
		t.Errorf("personal: got %q, want %q", tok2, "personal-tok")
	}

	// Delete one, other persists
	_ = store.DeleteToken("work")
	_, err := store.RetrieveToken("work")
	if err != keyring.ErrNotFound {
		t.Errorf("work should be deleted")
	}
	tok2, _ = store.RetrieveToken("personal")
	if tok2 != "personal-tok" {
		t.Errorf("personal should persist, got %q", tok2)
	}
}

func TestFileTokenStore_Overwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	store := &fileTokenStore{path: path}

	_ = store.StoreToken("default", "old")
	_ = store.StoreToken("default", "new")

	tok, _ := store.RetrieveToken("default")
	if tok != "new" {
		t.Errorf("got %q, want %q", tok, "new")
	}
}

func TestFileTokenStore_Permissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	store := &fileTokenStore{path: path}

	_ = store.StoreToken("default", "secret")

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("file permissions: got %o, want 600", perm)
	}
}

func TestFileTokenStore_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.json")
	store := &fileTokenStore{path: path}

	_ = store.StoreToken("default", "tok")

	// No leftover temp files
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "tokens.json" {
			t.Errorf("unexpected file: %s", e.Name())
		}
	}
}

func TestFileTokenStore_ValidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	store := &fileTokenStore{path: path}

	_ = store.StoreToken("default", "my-tok")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var data map[string]string
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["default-token"] != "my-tok" {
		t.Errorf("JSON key: got %v, want default-token=my-tok", data)
	}
}

func TestFileTokenStore_NestedDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deep", "nested", "tokens.json")
	store := &fileTokenStore{path: path}

	if err := store.StoreToken("default", "tok"); err != nil {
		t.Fatalf("StoreToken with nested dir: %v", err)
	}

	tok, _ := store.RetrieveToken("default")
	if tok != "tok" {
		t.Errorf("got %q, want %q", tok, "tok")
	}
}

// errKeyring is a mock TokenStore that always returns errors (simulating broken keyring).
type errKeyring struct{}

func (e *errKeyring) StoreToken(_, _ string) error { return errors.New("keyring: dbus unavailable") }
func (e *errKeyring) RetrieveToken(_ string) (string, error) {
	return "", errors.New("keyring: dbus unavailable")
}
func (e *errKeyring) DeleteToken(_ string) error { return errors.New("keyring: dbus unavailable") }

func TestFallbackStore_StoreWithKeyringFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	var stderr bytes.Buffer
	store := &fallbackStore{
		primary:  &errKeyring{},
		fallback: &fileTokenStore{path: path},
		stderr:   &stderr,
	}

	if err := store.StoreToken("default", "tok"); err != nil {
		t.Fatalf("StoreToken: %v", err)
	}

	// Warning printed
	if stderr.Len() == 0 {
		t.Error("expected warning on stderr")
	}
	if got := stderr.String(); !bytes.Contains([]byte(got), []byte("keyring unavailable")) {
		t.Errorf("warning missing 'keyring unavailable': %q", got)
	}

	// Token retrievable from fallback
	tok, err := store.RetrieveToken("default")
	if err != nil {
		t.Fatalf("RetrieveToken: %v", err)
	}
	if tok != "tok" {
		t.Errorf("got %q, want %q", tok, "tok")
	}
}

func TestFallbackStore_StoreWithKeyringSuccess(t *testing.T) {
	keyring.MockInit()
	path := filepath.Join(t.TempDir(), "tokens.json")
	var stderr bytes.Buffer
	store := &fallbackStore{
		primary:  &keyringStore{},
		fallback: &fileTokenStore{path: path},
		stderr:   &stderr,
	}

	if err := store.StoreToken("default", "tok"); err != nil {
		t.Fatalf("StoreToken: %v", err)
	}

	// No warning
	if stderr.Len() != 0 {
		t.Errorf("unexpected stderr: %q", stderr.String())
	}

	// Token NOT in file (went to keyring)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("fallback file should not exist when keyring works")
	}
}

func TestFallbackStore_RetrieveFromKeyring(t *testing.T) {
	keyring.MockInit()
	path := filepath.Join(t.TempDir(), "tokens.json")
	var stderr bytes.Buffer
	primary := &keyringStore{}
	store := &fallbackStore{
		primary:  primary,
		fallback: &fileTokenStore{path: path},
		stderr:   &stderr,
	}

	_ = primary.StoreToken("default", "keyring-tok")

	tok, err := store.RetrieveToken("default")
	if err != nil {
		t.Fatalf("RetrieveToken: %v", err)
	}
	if tok != "keyring-tok" {
		t.Errorf("got %q, want %q", tok, "keyring-tok")
	}
}

func TestFallbackStore_RetrieveNotFoundInBoth(t *testing.T) {
	keyring.MockInit()
	path := filepath.Join(t.TempDir(), "tokens.json")
	var stderr bytes.Buffer
	store := &fallbackStore{
		primary:  &keyringStore{},
		fallback: &fileTokenStore{path: path},
		stderr:   &stderr,
	}

	_, err := store.RetrieveToken("nonexistent")
	if err != keyring.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFallbackStore_DeleteFromBoth(t *testing.T) {
	keyring.MockInit()
	path := filepath.Join(t.TempDir(), "tokens.json")
	var stderr bytes.Buffer
	primary := &keyringStore{}
	fb := &fileTokenStore{path: path}
	store := &fallbackStore{
		primary:  primary,
		fallback: fb,
		stderr:   &stderr,
	}

	// Store in both
	_ = primary.StoreToken("default", "keyring-tok")
	_ = fb.StoreToken("default", "file-tok")

	if err := store.DeleteToken("default"); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}

	// Both should be gone
	_, err := primary.RetrieveToken("default")
	if err != keyring.ErrNotFound {
		t.Error("keyring token should be deleted")
	}
	_, err = fb.RetrieveToken("default")
	if err != keyring.ErrNotFound {
		t.Error("file token should be deleted")
	}
}

func TestFallbackStore_DeleteNotFoundInBoth(t *testing.T) {
	keyring.MockInit()
	path := filepath.Join(t.TempDir(), "tokens.json")
	var stderr bytes.Buffer
	store := &fallbackStore{
		primary:  &keyringStore{},
		fallback: &fileTokenStore{path: path},
		stderr:   &stderr,
	}

	err := store.DeleteToken("nonexistent")
	if err != keyring.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestNewTokenStore_Integration(t *testing.T) {
	keyring.MockInit()
	path := filepath.Join(t.TempDir(), "tokens.json")
	var stderr bytes.Buffer
	store := NewTokenStore(path, &stderr)

	// Full lifecycle via public constructor
	if err := store.StoreToken("prod", "prod-token"); err != nil {
		t.Fatalf("StoreToken: %v", err)
	}

	tok, err := store.RetrieveToken("prod")
	if err != nil {
		t.Fatalf("RetrieveToken: %v", err)
	}
	if tok != "prod-token" {
		t.Errorf("got %q, want %q", tok, "prod-token")
	}

	if err := store.DeleteToken("prod"); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}

	_, err = store.RetrieveToken("prod")
	if err != keyring.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestTokenKey(t *testing.T) {
	tests := []struct {
		profile string
		want    string
	}{
		{"default", "default-token"},
		{"work", "work-token"},
		{"my-profile", "my-profile-token"},
	}
	for _, tt := range tests {
		if got := tokenKey(tt.profile); got != tt.want {
			t.Errorf("tokenKey(%q) = %q, want %q", tt.profile, got, tt.want)
		}
	}
}
