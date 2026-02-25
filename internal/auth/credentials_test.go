package auth

import (
	"errors"
	"testing"

	cliErrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/zalando/go-keyring"
)

// mockProfileConfig implements ProfileConfig for tests.
type mockProfileConfig struct {
	active   string
	profiles map[string]*ProfileData
}

func (m *mockProfileConfig) ActiveProfile() string {
	if m.active != "" {
		return m.active
	}
	return "default"
}

func (m *mockProfileConfig) GetProfile(name string) *ProfileData {
	return m.profiles[name]
}

// mockTokenStore implements TokenStore for tests.
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

func TestResolve(t *testing.T) {
	tests := []struct {
		name         string
		flagInstance string
		flagUser     string
		flagToken    string
		profileName  string
		envVars      map[string]string
		cfg          ProfileConfig
		tokenStore   TokenStore
		wantCreds    *Credentials
		wantErrCode  cliErrors.ErrorCode
		wantErrMsg   string
	}{
		{
			name:         "flags: all three provided",
			flagInstance: "mysite.atlassian.net",
			flagUser:     "user@example.com",
			flagToken:    "tok123",
			cfg:          &mockProfileConfig{},
			tokenStore:   &mockTokenStore{tokens: map[string]string{}},
			wantCreds: &Credentials{
				Instance: "mysite.atlassian.net",
				User:     "user@example.com",
				Token:    "tok123",
			},
		},
		{
			name:         "flags: URL normalization strips scheme",
			flagInstance: "https://mysite.atlassian.net",
			flagUser:     "user@example.com",
			flagToken:    "tok123",
			cfg:          &mockProfileConfig{},
			tokenStore:   &mockTokenStore{tokens: map[string]string{}},
			wantCreds: &Credentials{
				Instance: "mysite.atlassian.net",
				User:     "user@example.com",
				Token:    "tok123",
			},
		},
		{
			name:         "flags: URL normalization strips trailing slash and rest path",
			flagInstance: "https://mysite.atlassian.net/rest/api/3/",
			flagUser:     "user@example.com",
			flagToken:    "tok123",
			cfg:          &mockProfileConfig{},
			tokenStore:   &mockTokenStore{tokens: map[string]string{}},
			wantCreds: &Credentials{
				Instance: "mysite.atlassian.net",
				User:     "user@example.com",
				Token:    "tok123",
			},
		},
		{
			name:         "flags: partial - missing token",
			flagInstance: "mysite.atlassian.net",
			flagUser:     "user@example.com",
			flagToken:    "",
			cfg:          &mockProfileConfig{},
			tokenStore:   &mockTokenStore{tokens: map[string]string{}},
			wantErrCode:  cliErrors.VALIDATION_ERROR,
			wantErrMsg:   "All three flags required",
		},
		{
			name:         "flags: partial - missing user",
			flagInstance: "mysite.atlassian.net",
			flagUser:     "",
			flagToken:    "tok123",
			cfg:          &mockProfileConfig{},
			tokenStore:   &mockTokenStore{tokens: map[string]string{}},
			wantErrCode:  cliErrors.VALIDATION_ERROR,
			wantErrMsg:   "All three flags required",
		},
		{
			name:         "flags: partial - missing instance",
			flagInstance: "",
			flagUser:     "user@example.com",
			flagToken:    "tok123",
			cfg:          &mockProfileConfig{},
			tokenStore:   &mockTokenStore{tokens: map[string]string{}},
			wantErrCode:  cliErrors.VALIDATION_ERROR,
			wantErrMsg:   "All three flags required",
		},
		{
			name:       "env: all three set",
			envVars:    map[string]string{"JIRA_INSTANCE": "env-site.atlassian.net", "JIRA_USER": "env@example.com", "JIRA_TOKEN": "envtok"},
			cfg:        &mockProfileConfig{},
			tokenStore: &mockTokenStore{tokens: map[string]string{}},
			wantCreds: &Credentials{
				Instance: "env-site.atlassian.net",
				User:     "env@example.com",
				Token:    "envtok",
			},
		},
		{
			name:       "env: URL normalization",
			envVars:    map[string]string{"JIRA_INSTANCE": "http://env-site.atlassian.net/", "JIRA_USER": "env@example.com", "JIRA_TOKEN": "envtok"},
			cfg:        &mockProfileConfig{},
			tokenStore: &mockTokenStore{tokens: map[string]string{}},
			wantCreds: &Credentials{
				Instance: "env-site.atlassian.net",
				User:     "env@example.com",
				Token:    "envtok",
			},
		},
		{
			name:        "env: partial - missing token",
			envVars:     map[string]string{"JIRA_INSTANCE": "env-site.atlassian.net", "JIRA_USER": "env@example.com"},
			cfg:         &mockProfileConfig{},
			tokenStore:  &mockTokenStore{tokens: map[string]string{}},
			wantErrCode: cliErrors.VALIDATION_ERROR,
			wantErrMsg:  "All three env vars required",
		},
		{
			name:        "env: partial - missing user",
			envVars:     map[string]string{"JIRA_INSTANCE": "env-site.atlassian.net", "JIRA_TOKEN": "envtok"},
			cfg:         &mockProfileConfig{},
			tokenStore:  &mockTokenStore{tokens: map[string]string{}},
			wantErrCode: cliErrors.VALIDATION_ERROR,
			wantErrMsg:  "All three env vars required",
		},
		{
			name: "profile: active profile resolved",
			cfg: &mockProfileConfig{
				active: "work",
				profiles: map[string]*ProfileData{
					"work": {Name: "work", Instance: "work.atlassian.net", User: "work@example.com"},
				},
			},
			tokenStore: &mockTokenStore{tokens: map[string]string{"work": "worktok"}},
			wantCreds: &Credentials{
				Instance: "work.atlassian.net",
				User:     "work@example.com",
				Token:    "worktok",
			},
		},
		{
			name:        "profile: named profile via --profile",
			profileName: "staging",
			cfg: &mockProfileConfig{
				active: "work",
				profiles: map[string]*ProfileData{
					"work":    {Name: "work", Instance: "work.atlassian.net", User: "work@example.com"},
					"staging": {Name: "staging", Instance: "staging.atlassian.net", User: "staging@example.com"},
				},
			},
			tokenStore: &mockTokenStore{tokens: map[string]string{"work": "worktok", "staging": "stagingtok"}},
			wantCreds: &Credentials{
				Instance: "staging.atlassian.net",
				User:     "staging@example.com",
				Token:    "stagingtok",
			},
		},
		{
			name: "profile: URL normalization on stored profile",
			cfg: &mockProfileConfig{
				active: "default",
				profiles: map[string]*ProfileData{
					"default": {Name: "default", Instance: "https://stored.atlassian.net/", User: "user@example.com"},
				},
			},
			tokenStore: &mockTokenStore{tokens: map[string]string{"default": "tok"}},
			wantCreds: &Credentials{
				Instance: "stored.atlassian.net",
				User:     "user@example.com",
				Token:    "tok",
			},
		},
		{
			name:        "profile: profile not found",
			cfg:         &mockProfileConfig{profiles: map[string]*ProfileData{}},
			tokenStore:  &mockTokenStore{tokens: map[string]string{}},
			wantErrCode: cliErrors.AUTH_ERROR,
			wantErrMsg:  "No credentials found",
		},
		{
			name: "profile: token not found in store",
			cfg: &mockProfileConfig{
				active: "default",
				profiles: map[string]*ProfileData{
					"default": {Name: "default", Instance: "mysite.atlassian.net", User: "user@example.com"},
				},
			},
			tokenStore:  &mockTokenStore{tokens: map[string]string{}},
			wantErrCode: cliErrors.AUTH_ERROR,
			wantErrMsg:  "No credentials found",
		},
		{
			name:        "no credentials at all: nil config",
			cfg:         nil,
			tokenStore:  &mockTokenStore{tokens: map[string]string{}},
			wantErrCode: cliErrors.AUTH_ERROR,
			wantErrMsg:  "No credentials found",
		},
		{
			name:        "no credentials at all: empty config",
			cfg:         &mockProfileConfig{profiles: map[string]*ProfileData{}},
			tokenStore:  &mockTokenStore{tokens: map[string]string{}},
			wantErrCode: cliErrors.AUTH_ERROR,
			wantErrMsg:  "No credentials found",
		},
		{
			name:         "flags override env",
			flagInstance: "flag-site.atlassian.net",
			flagUser:     "flag@example.com",
			flagToken:    "flagtok",
			envVars:      map[string]string{"JIRA_INSTANCE": "env-site.atlassian.net", "JIRA_USER": "env@example.com", "JIRA_TOKEN": "envtok"},
			cfg: &mockProfileConfig{
				active: "default",
				profiles: map[string]*ProfileData{
					"default": {Name: "default", Instance: "stored.atlassian.net", User: "stored@example.com"},
				},
			},
			tokenStore: &mockTokenStore{tokens: map[string]string{"default": "storedtok"}},
			wantCreds: &Credentials{
				Instance: "flag-site.atlassian.net",
				User:     "flag@example.com",
				Token:    "flagtok",
			},
		},
		{
			name:    "env overrides profile",
			envVars: map[string]string{"JIRA_INSTANCE": "env-site.atlassian.net", "JIRA_USER": "env@example.com", "JIRA_TOKEN": "envtok"},
			cfg: &mockProfileConfig{
				active: "default",
				profiles: map[string]*ProfileData{
					"default": {Name: "default", Instance: "stored.atlassian.net", User: "stored@example.com"},
				},
			},
			tokenStore: &mockTokenStore{tokens: map[string]string{"default": "storedtok"}},
			wantCreds: &Credentials{
				Instance: "env-site.atlassian.net",
				User:     "env@example.com",
				Token:    "envtok",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear env vars before each test.
			t.Setenv("JIRA_INSTANCE", "")
			t.Setenv("JIRA_USER", "")
			t.Setenv("JIRA_TOKEN", "")

			// Set test-specific env vars.
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			creds, err := Resolve(tt.flagInstance, tt.flagUser, tt.flagToken, tt.profileName, tt.cfg, tt.tokenStore)

			if tt.wantErrCode != "" {
				if err == nil {
					t.Fatalf("expected error with code %s, got nil", tt.wantErrCode)
				}
				var cliErr *cliErrors.CLIError
				if !errors.As(err, &cliErr) {
					t.Fatalf("expected CLIError, got %T: %v", err, err)
				}
				if cliErr.Code != tt.wantErrCode {
					t.Errorf("error code = %s, want %s", cliErr.Code, tt.wantErrCode)
				}
				if tt.wantErrMsg != "" && !contains(cliErr.Message, tt.wantErrMsg) {
					t.Errorf("error message = %q, want to contain %q", cliErr.Message, tt.wantErrMsg)
				}
				if cliErr.Suggestion == "" {
					t.Error("expected non-empty suggestion on error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if creds.Instance != tt.wantCreds.Instance {
				t.Errorf("Instance = %q, want %q", creds.Instance, tt.wantCreds.Instance)
			}
			if creds.User != tt.wantCreds.User {
				t.Errorf("User = %q, want %q", creds.User, tt.wantCreds.User)
			}
			if creds.Token != tt.wantCreds.Token {
				t.Errorf("Token = %q, want %q", creds.Token, tt.wantCreds.Token)
			}
		})
	}
}

func TestNormalizeInstanceURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"mysite.atlassian.net", "mysite.atlassian.net"},
		{"https://mysite.atlassian.net", "mysite.atlassian.net"},
		{"http://mysite.atlassian.net", "mysite.atlassian.net"},
		{"https://mysite.atlassian.net/", "mysite.atlassian.net"},
		{"https://mysite.atlassian.net///", "mysite.atlassian.net"},
		{"https://mysite.atlassian.net/rest/api/3/issue", "mysite.atlassian.net"},
		{"mysite.atlassian.net/rest/api/3/", "mysite.atlassian.net"},
		{"https://mysite.atlassian.net/rest/", "mysite.atlassian.net"},
		{"http://localhost:8080", "localhost:8080"},
		{"http://localhost:8080/", "localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeInstanceURL(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeInstanceURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// contains checks if s contains substr (simple helper for test assertions).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
