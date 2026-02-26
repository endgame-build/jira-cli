package factory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/auth"
	"github.com/endgameio/jira-cli/internal/config"
	"github.com/endgameio/jira-cli/internal/iostreams"
	"github.com/zalando/go-keyring"
)

func TestNew(t *testing.T) {
	f := New()
	if f.IOStreams == nil {
		t.Fatal("expected IOStreams to be non-nil")
	}
	if f.Profile != "" || f.OutputJSON || f.Quiet || f.DryRun {
		t.Error("expected global flags to be zero-valued")
	}
}

func TestConfigLazy(t *testing.T) {
	tio := iostreams.Test()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	f := NewTestFactory(tio.IOStreams, cfg, nil)

	got, err := f.Config()
	if err != nil {
		t.Fatalf("Config() error: %v", err)
	}
	if got == nil {
		t.Fatal("Config() returned nil")
	}

	// Second call returns same instance (cached via sync.Once).
	got2, _ := f.Config()
	if got != got2 {
		t.Error("expected Config() to return cached instance")
	}
}

func TestConfigLazy_RealLoad(t *testing.T) {
	f := &Factory{
		IOStreams: iostreams.Test().IOStreams,
	}
	// Calls config.Load() — should not error on missing file.
	cfg, err := f.Config()
	if err != nil {
		t.Fatalf("Config() error on real load: %v", err)
	}
	if cfg == nil {
		t.Fatal("Config() returned nil on real load")
	}
}

func TestAPIClientLazy_NotCalledUntilAccessed(t *testing.T) {
	tio := iostreams.Test()
	f := &Factory{
		IOStreams: tio.IOStreams,
	}

	// Auth-free path: only call Config().
	_, err := f.Config()
	if err != nil {
		t.Fatalf("Config() error: %v", err)
	}

	// APIClient should NOT have been initialized.
	if f.clientVal != nil {
		t.Error("expected APIClient to NOT be initialized before access")
	}
}

func TestAPIClientLazy_ResolvedOnAccess(t *testing.T) {
	keyring.MockInit()

	tio := iostreams.Test()

	// Write config with a profile to disk.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	tomlData := `active_profile = "default"

[profiles.default]
instance = "mysite.atlassian.net"
user = "user@example.com"
`
	if err := os.WriteFile(cfgPath, []byte(tomlData), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	tokenStore := &mockTokenStore{tokens: map[string]string{"default": "test-token"}}

	f := &Factory{
		IOStreams: tio.IOStreams,
	}
	f.configOnce.Do(func() {
		f.configVal = cfg
	})
	f.tokenStoreOnce.Do(func() {
		f.tokenStoreVal = tokenStore
	})

	client, err := f.APIClient()
	if err != nil {
		t.Fatalf("APIClient() error: %v", err)
	}
	if client == nil {
		t.Fatal("APIClient() returned nil")
	}
	if client.Instance() != "mysite.atlassian.net" {
		t.Errorf("got instance %q, want %q", client.Instance(), "mysite.atlassian.net")
	}
}

func TestAPIClientLazy_FlagCredentials(t *testing.T) {
	tio := iostreams.Test()
	f := &Factory{
		IOStreams:    tio.IOStreams,
		FlagInstance: "flag-site.atlassian.net",
		FlagUser:     "flag@example.com",
		FlagToken:    "flag-token",
	}

	client, err := f.APIClient()
	if err != nil {
		t.Fatalf("APIClient() error: %v", err)
	}
	if client.Instance() != "flag-site.atlassian.net" {
		t.Errorf("got instance %q, want %q", client.Instance(), "flag-site.atlassian.net")
	}
}

func TestAPIClientLazy_EnvCredentials(t *testing.T) {
	tio := iostreams.Test()
	t.Setenv("JIRA_INSTANCE", "https://env-site.atlassian.net")
	t.Setenv("JIRA_USER", "env@example.com")
	t.Setenv("JIRA_TOKEN", "env-token")

	f := &Factory{
		IOStreams: tio.IOStreams,
	}

	client, err := f.APIClient()
	if err != nil {
		t.Fatalf("APIClient() error: %v", err)
	}
	if client.Instance() != "env-site.atlassian.net" {
		t.Errorf("got instance %q, want %q", client.Instance(), "env-site.atlassian.net")
	}
}

func TestAPIClientLazy_NoCredentialsError(t *testing.T) {
	tio := iostreams.Test()
	t.Setenv("JIRA_INSTANCE", "")
	t.Setenv("JIRA_USER", "")
	t.Setenv("JIRA_TOKEN", "")

	f := &Factory{
		IOStreams: tio.IOStreams,
	}

	_, err := f.APIClient()
	if err == nil {
		t.Fatal("expected error when no credentials available")
	}
}

func TestAuthCredentials_CachedOnSecondCall(t *testing.T) {
	tio := iostreams.Test()
	f := &Factory{
		IOStreams:    tio.IOStreams,
		FlagInstance: "site.atlassian.net",
		FlagUser:     "user@example.com",
		FlagToken:    "token",
	}

	creds1, err := f.AuthCredentials()
	if err != nil {
		t.Fatalf("first AuthCredentials() error: %v", err)
	}

	creds2, _ := f.AuthCredentials()
	if creds1 != creds2 {
		t.Error("expected AuthCredentials() to return cached instance")
	}
}

func TestTokenStore(t *testing.T) {
	tio := iostreams.Test()
	f := &Factory{
		IOStreams: tio.IOStreams,
	}

	ts := f.TokenStore()
	if ts == nil {
		t.Fatal("TokenStore() returned nil")
	}

	ts2 := f.TokenStore()
	if ts != ts2 {
		t.Error("expected TokenStore() to return cached instance")
	}
}

func TestNewTestFactory(t *testing.T) {
	tio := iostreams.Test()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg, _ := config.LoadFromPath(cfgPath)

	creds := &auth.Credentials{Instance: "test.atlassian.net", User: "u", Token: "t"}
	client := api.NewClient(creds)

	f := NewTestFactory(tio.IOStreams, cfg, client)

	if f.IOStreams != tio.IOStreams {
		t.Error("IOStreams mismatch")
	}

	gotCfg, err := f.Config()
	if err != nil {
		t.Fatalf("Config() error: %v", err)
	}
	if gotCfg != cfg {
		t.Error("Config() returned different instance")
	}

	gotClient, err := f.APIClient()
	if err != nil {
		t.Fatalf("APIClient() error: %v", err)
	}
	if gotClient != client {
		t.Error("APIClient() returned different instance")
	}
}

func TestNewTestFactory_NilClient(t *testing.T) {
	tio := iostreams.Test()
	f := NewTestFactory(tio.IOStreams, nil, nil)

	gotCfg, err := f.Config()
	if err != nil {
		t.Fatalf("Config() error: %v", err)
	}
	if gotCfg != nil {
		t.Error("expected Config() to return nil when pre-filled with nil")
	}
}

func TestProfileConfigBridge(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	tomlData := `active_profile = "work"

[profiles.work]
instance = "work.atlassian.net"
user = "work@example.com"
`
	if err := os.WriteFile(cfgPath, []byte(tomlData), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	fc, ok := cfg.(profileConfigAdapter)
	if !ok {
		t.Fatal("config does not implement profileConfigAdapter")
	}

	bridge := &profileConfigBridge{cfg: fc}

	if got := bridge.ActiveProfile(); got != "work" {
		t.Errorf("ActiveProfile() = %q, want %q", got, "work")
	}

	pd := bridge.GetProfile("work")
	if pd == nil {
		t.Fatal("GetProfile(work) returned nil")
	}
	if pd.Name != "work" || pd.Instance != "work.atlassian.net" || pd.User != "work@example.com" {
		t.Errorf("profile data mismatch: %+v", pd)
	}

	if bridge.GetProfile("nonexistent") != nil {
		t.Error("expected nil for nonexistent profile")
	}
}

func TestConcurrentAccess(t *testing.T) {
	tio := iostreams.Test()
	f := &Factory{
		IOStreams:    tio.IOStreams,
		FlagInstance: "site.atlassian.net",
		FlagUser:     "user@example.com",
		FlagToken:    "token",
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = f.Config()
			_, _ = f.AuthCredentials()
			_, _ = f.APIClient()
		}()
	}
	wg.Wait()

	client, err := f.APIClient()
	if err != nil {
		t.Fatalf("APIClient() error after concurrent access: %v", err)
	}
	if client == nil {
		t.Fatal("APIClient() nil after concurrent access")
	}
}

func TestAPIClient_UsableForRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("expected Authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"key": "PROJ-1"})
	}))
	defer srv.Close()

	creds := &auth.Credentials{Instance: "test.atlassian.net", User: "u@test.com", Token: "tok"}
	client := api.NewClient(creds, api.WithBaseURL(srv.URL+"/rest/api/3"))

	tio := iostreams.Test()
	f := NewTestFactory(tio.IOStreams, nil, client)

	c, err := f.APIClient()
	if err != nil {
		t.Fatalf("APIClient() error: %v", err)
	}

	var result map[string]string
	err = c.Do(context.Background(), "GET", "issue/PROJ-1", nil, &result)
	if err != nil {
		t.Fatalf("Do() error: %v", err)
	}
	if result["key"] != "PROJ-1" {
		t.Errorf("got key %q, want %q", result["key"], "PROJ-1")
	}
}

func TestGlobalFlagsZeroValue(t *testing.T) {
	f := &Factory{}
	if f.Profile != "" {
		t.Error("Profile should be empty")
	}
	if f.FlagInstance != "" {
		t.Error("FlagInstance should be empty")
	}
	if f.FlagUser != "" {
		t.Error("FlagUser should be empty")
	}
	if f.FlagToken != "" {
		t.Error("FlagToken should be empty")
	}
	if f.OutputJSON {
		t.Error("OutputJSON should be false")
	}
	if f.NoColor {
		t.Error("NoColor should be false")
	}
	if f.Verbose {
		t.Error("Verbose should be false")
	}
	if f.DryRun {
		t.Error("DryRun should be false")
	}
	if f.Quiet {
		t.Error("Quiet should be false")
	}
	if f.JQExpr != "" {
		t.Error("JQExpr should be empty")
	}
	if f.Text {
		t.Error("Text should be false")
	}
}

// mockTokenStore implements auth.TokenStore for tests.
type mockTokenStore struct {
	tokens map[string]string
}

func (m *mockTokenStore) StoreToken(profile, token string) error {
	m.tokens[profile] = token
	return nil
}

func (m *mockTokenStore) RetrieveToken(profile string) (string, error) {
	tok, ok := m.tokens[profile]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return tok, nil
}

func (m *mockTokenStore) DeleteToken(profile string) error {
	delete(m.tokens, profile)
	return nil
}
