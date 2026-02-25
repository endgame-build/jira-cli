// Package config provides persistent TOML configuration with XDG paths and
// profile support for managing multiple Jira instances.
package config

import (
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

const appName = "jira-cli"

// configDir returns the XDG config directory for the application.
// Override via $XDG_CONFIG_HOME; defaults to ~/.config/jira-cli (Linux)
// or ~/Library/Application Support/jira-cli (macOS).
func configDir() string {
	return filepath.Join(xdg.ConfigHome, appName)
}

// configFilePath returns the full path to config.toml.
func configFilePath() string {
	return filepath.Join(configDir(), "config.toml")
}

// ensureConfigDir creates the config directory if it does not exist.
func ensureConfigDir() error {
	return os.MkdirAll(configDir(), 0o755)
}
