package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config provides read/write access to persistent CLI configuration.
type Config interface {
	Get(key string) string
	Set(key, value string) error
	Delete(key string) error
	List() map[string]string
	Save() error
}

// validKeys enumerates the allowed dot-separated config keys.
var validKeys = map[string]bool{
	"default.project":  true,
	"default.assignee": true,
	"output.format":    true,
	"output.color":     true,
}

// validFormats are the allowed values for output.format.
var validFormats = map[string]bool{
	"text": true,
	"json": true,
}

// validColors are the allowed values for output.color.
var validColors = map[string]bool{
	"auto":   true,
	"always": true,
	"never":  true,
}

// tomlConfig is the on-disk representation of config.toml.
type tomlConfig struct {
	Defaults tomlDefaults           `toml:"defaults,omitempty"`
	Output   tomlOutput             `toml:"output,omitempty"`
	Aliases  map[string]string      `toml:"aliases,omitempty"`
	Profiles map[string]tomlProfile `toml:"profiles,omitempty"`
	Active   string                 `toml:"active_profile,omitempty"`
}

type tomlDefaults struct {
	Project  string `toml:"project,omitempty"`
	Assignee string `toml:"assignee,omitempty"`
}

type tomlOutput struct {
	Format string `toml:"format,omitempty"`
	Color  string `toml:"color,omitempty"`
}

// tomlProfile stores per-profile metadata (no secrets — tokens live in keyring).
type tomlProfile struct {
	Instance string `toml:"instance"`
	User     string `toml:"user"`
}

// fileConfig is the in-memory Config backed by a TOML file.
type fileConfig struct {
	data tomlConfig
	path string
}

// Load reads config from the XDG path. Missing file returns default config.
func Load() (Config, error) {
	return loadFromPath(configFilePath())
}

// LoadFromPath reads config from a specific path (useful for testing).
func LoadFromPath(path string) (Config, error) {
	return loadFromPath(path)
}

func loadFromPath(path string) (Config, error) {
	cfg := &fileConfig{
		path: path,
		data: tomlConfig{
			Aliases:  make(map[string]string),
			Profiles: make(map[string]tomlProfile),
		},
	}

	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	if err := toml.Unmarshal(raw, &cfg.data); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Ensure maps are non-nil after unmarshal.
	if cfg.data.Aliases == nil {
		cfg.data.Aliases = make(map[string]string)
	}
	if cfg.data.Profiles == nil {
		cfg.data.Profiles = make(map[string]tomlProfile)
	}

	return cfg, nil
}

// Get returns the value for a dot-separated key, or empty string if unset.
func (c *fileConfig) Get(key string) string {
	switch key {
	case "default.project":
		return c.data.Defaults.Project
	case "default.assignee":
		return c.data.Defaults.Assignee
	case "output.format":
		return c.data.Output.Format
	case "output.color":
		return c.data.Output.Color
	default:
		return ""
	}
}

// Set stores a value for a dot-separated key. Returns error on invalid key or value.
func (c *fileConfig) Set(key, value string) error {
	if !validKeys[key] {
		return fmt.Errorf("unknown config key: %s (valid keys: %s)", key, validKeysList())
	}

	if err := validateValue(key, value); err != nil {
		return err
	}

	switch key {
	case "default.project":
		c.data.Defaults.Project = value
	case "default.assignee":
		c.data.Defaults.Assignee = value
	case "output.format":
		c.data.Output.Format = value
	case "output.color":
		c.data.Output.Color = value
	}

	return nil
}

// Delete removes a config key by resetting it to empty.
func (c *fileConfig) Delete(key string) error {
	if !validKeys[key] {
		return fmt.Errorf("unknown config key: %s", key)
	}
	return c.Set(key, "")
}

// List returns all set key-value pairs.
func (c *fileConfig) List() map[string]string {
	result := make(map[string]string)
	for key := range validKeys {
		if v := c.Get(key); v != "" {
			result[key] = v
		}
	}
	return result
}

// Save writes the config to disk using write-to-temp-then-rename for atomicity.
func (c *fileConfig) Save() error {
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	raw, err := toml.Marshal(c.data)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	// Write-to-temp-then-rename: atomic on POSIX, best-effort on Windows.
	tmp, err := os.CreateTemp(dir, "config-*.toml.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, c.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming config file: %w", err)
	}

	return nil
}

// validateValue checks value constraints for specific keys.
func validateValue(key, value string) error {
	switch key {
	case "output.format":
		if value != "" && !validFormats[value] {
			return fmt.Errorf("invalid value for output.format: %q (valid: text, json)", value)
		}
	case "output.color":
		if value != "" && !validColors[value] {
			return fmt.Errorf("invalid value for output.color: %q (valid: auto, always, never)", value)
		}
	}
	return nil
}

// validKeysList returns a sorted comma-separated list of valid keys.
func validKeysList() string {
	keys := make([]string, 0, len(validKeys))
	for k := range validKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
