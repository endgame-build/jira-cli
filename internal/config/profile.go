package config

import (
	"fmt"
	"sort"
)

// Profile represents a named credential set for a Jira instance.
// Tokens are stored separately in the keyring (see internal/auth).
type Profile struct {
	Name     string
	Instance string
	User     string
}

// ActiveProfile returns the name of the active profile, or "default" if unset.
func (c *fileConfig) ActiveProfile() string {
	if c.data.Active != "" {
		return c.data.Active
	}
	return "default"
}

// SetActiveProfile sets the active profile name and saves.
func (c *fileConfig) SetActiveProfile(name string) error {
	if _, ok := c.data.Profiles[name]; !ok {
		return fmt.Errorf("profile not found: %s", name)
	}
	c.data.Active = name
	return nil
}

// GetProfile returns a profile by name, or nil if not found.
func (c *fileConfig) GetProfile(name string) *Profile {
	p, ok := c.data.Profiles[name]
	if !ok {
		return nil
	}
	return &Profile{
		Name:     name,
		Instance: p.Instance,
		User:     p.User,
	}
}

// SetProfile creates or updates a profile with the given instance and user.
func (c *fileConfig) SetProfile(name, instance, user string) {
	c.data.Profiles[name] = tomlProfile{
		Instance: instance,
		User:     user,
	}
}

// DeleteProfile removes a profile. If it was the active profile, clears active.
func (c *fileConfig) DeleteProfile(name string) error {
	if _, ok := c.data.Profiles[name]; !ok {
		return fmt.Errorf("profile not found: %s", name)
	}
	delete(c.data.Profiles, name)
	if c.data.Active == name {
		c.data.Active = ""
	}
	return nil
}

// ListProfiles returns all profile names sorted alphabetically.
func (c *fileConfig) ListProfiles() []string {
	names := make([]string, 0, len(c.data.Profiles))
	for name := range c.data.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Aliases returns the alias map.
func (c *fileConfig) Aliases() map[string]string {
	return c.data.Aliases
}

// SetAlias stores an alias mapping.
func (c *fileConfig) SetAlias(name, command string) {
	c.data.Aliases[name] = command
}

// DeleteAlias removes an alias.
func (c *fileConfig) DeleteAlias(name string) {
	delete(c.data.Aliases, name)
}
