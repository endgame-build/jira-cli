// Package factory provides the dependency injection hub for jira-cli commands.
// IOStreams is eagerly initialized; Config, Auth, and APIClient are lazy.
package factory

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/auth"
	"github.com/endgameio/jira-cli/internal/config"
	"github.com/endgameio/jira-cli/internal/iostreams"
)

// Factory is the dependency injection hub for all CLI commands.
// IOStreams is always available; Config, AuthResolver, and APIClient
// are initialized lazily on first access so that auth-free commands
// (--help, config, alias) never trigger credential resolution.
type Factory struct {
	IOStreams *iostreams.IOStreams

	// Global flag values — set by PersistentPreRunE, read by commands.
	Profile      string // --profile
	FlagInstance string // --instance
	FlagUser     string // --user
	FlagToken    string // --token
	OutputJSON   bool   // --json
	NoColor      bool   // --no-color
	Verbose      bool   // --verbose (no-op, stored only)
	DryRun       bool   // --dry-run
	Quiet        bool   // --quiet
	JQExpr       string // --jq
	Text         bool   // --text

	// Lazy accessors — guarded by sync.Once.
	configOnce sync.Once
	configVal  config.Config
	configErr  error

	tokenStoreOnce sync.Once
	tokenStoreVal  auth.TokenStore

	authOnce sync.Once
	authVal  *auth.Credentials
	authErr  error

	clientOnce sync.Once
	clientVal  *api.Client
	clientErr  error
}

// New creates a Factory with real IOStreams. Called once in main.go.
func New() *Factory {
	return &Factory{
		IOStreams: iostreams.New(),
	}
}

// Config returns the loaded configuration, initializing on first call.
func (f *Factory) Config() (config.Config, error) {
	f.configOnce.Do(func() {
		f.configVal, f.configErr = config.Load()
	})
	return f.configVal, f.configErr
}

// TokenStore returns the credential store, initializing on first call.
func (f *Factory) TokenStore() auth.TokenStore {
	f.tokenStoreOnce.Do(func() {
		tokensPath := filepath.Join(config.ConfigDir(), "tokens.json")
		f.tokenStoreVal = auth.NewTokenStore(tokensPath, f.IOStreams.Err)
	})
	return f.tokenStoreVal
}

// AuthCredentials resolves authentication credentials, initializing on first call.
// Uses the flag > env > profile chain from auth.Resolve().
func (f *Factory) AuthCredentials() (*auth.Credentials, error) {
	f.authOnce.Do(func() {
		cfg, err := f.Config()
		if err != nil {
			f.authErr = err
			return
		}

		// Type-assert to access profile methods (ActiveProfile, GetProfile).
		// The Config interface is narrow; profile methods live on *fileConfig.
		var profileCfg auth.ProfileConfig
		if pc, ok := cfg.(profileConfigAdapter); ok {
			profileCfg = &profileConfigBridge{pc}
		}

		f.authVal, f.authErr = auth.Resolve(
			f.FlagInstance, f.FlagUser, f.FlagToken,
			f.Profile, profileCfg, f.TokenStore(),
		)
	})
	return f.authVal, f.authErr
}

// APIClient returns a configured Jira API client, initializing on first call.
func (f *Factory) APIClient() (*api.Client, error) {
	f.clientOnce.Do(func() {
		creds, err := f.AuthCredentials()
		if err != nil {
			f.clientErr = err
			return
		}
		f.clientVal = api.NewClient(creds)
	})
	return f.clientVal, f.clientErr
}

// ResolveInstance returns the Jira instance URL using the same resolution chain
// as auth (flags → env → config profile) but WITHOUT triggering credential or
// token resolution. Returns empty string if none is configured.
func (f *Factory) ResolveInstance() string {
	// 1. Flag
	if f.FlagInstance != "" {
		return f.FlagInstance
	}
	// 2. Env var
	if env := os.Getenv("JIRA_INSTANCE"); env != "" {
		return env
	}
	// 3. Config profile
	cfg, err := f.Config()
	if err != nil || cfg == nil {
		return ""
	}
	pc, ok := cfg.(profileConfigAdapter)
	if !ok {
		return ""
	}
	profileName := f.Profile
	if profileName == "" {
		profileName = pc.ActiveProfile()
	}
	p := pc.GetProfile(profileName)
	if p == nil {
		return ""
	}
	return p.Instance
}

// profileConfigAdapter is the subset of methods we need from config's concrete type.
// This avoids exposing config.fileConfig publicly.
type profileConfigAdapter interface {
	ActiveProfile() string
	GetProfile(name string) *config.Profile
}

// profileConfigBridge adapts config's profile methods to auth.ProfileConfig.
type profileConfigBridge struct {
	cfg profileConfigAdapter
}

func (b *profileConfigBridge) ActiveProfile() string {
	return b.cfg.ActiveProfile()
}

func (b *profileConfigBridge) GetProfile(name string) *auth.ProfileData {
	p := b.cfg.GetProfile(name)
	if p == nil {
		return nil
	}
	return &auth.ProfileData{
		Name:     p.Name,
		Instance: p.Instance,
		User:     p.User,
	}
}
