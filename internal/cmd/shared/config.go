package shared

import (
	"fmt"
	"io"

	"github.com/endgame-build/jira-cli/internal/config"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
)

// ConfigProvider supplies config and a stderr writer for warnings.
// Satisfied by *factory.Factory.
type ConfigProvider interface {
	Config() (config.Config, error)
	Stderr() io.Writer
}

// ConfigGet safely retrieves a config value. Returns "" if missing.
// Warns on stderr if config cannot be loaded.
func ConfigGet(cp ConfigProvider, key string) string {
	cfg, err := cp.Config()
	if err != nil {
		fmt.Fprintf(cp.Stderr(), "Warning: could not load config: %v\n", err)
		return ""
	}
	if cfg == nil {
		return ""
	}
	return cfg.Get(key)
}

// ResolveProject resolves the project key from flag, config, or returns an error.
func ResolveProject(cp ConfigProvider, flagProject string) (string, error) {
	if flagProject != "" {
		return flagProject, nil
	}

	defaultProject := ConfigGet(cp, "default.project")
	if defaultProject != "" {
		return defaultProject, nil
	}

	return "", clierrors.NewValidationError("--project is required").
		WithSuggestion("Specify --project or set a default: jira config set default.project PROJ")
}
