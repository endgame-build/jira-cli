package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent", "config.toml")
	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("Load missing file: %v", err)
	}
	if got := cfg.Get("default.project"); got != "" {
		t.Errorf("expected empty default.project, got %q", got)
	}
}

func TestSetGetRoundTrip(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{"default.project", "PROJ"},
		{"default.assignee", "alice@example.com"},
		{"output.format", "json"},
		{"output.color", "never"},
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	for _, tt := range tests {
		if err := cfg.Set(tt.key, tt.value); err != nil {
			t.Errorf("Set(%q, %q): %v", tt.key, tt.value, err)
			continue
		}
		if got := cfg.Get(tt.key); got != tt.value {
			t.Errorf("Get(%q) = %q, want %q", tt.key, got, tt.value)
		}
	}
}

func TestSetInvalidKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if err := cfg.Set("bogus.key", "value"); err == nil {
		t.Error("expected error for invalid key, got nil")
	}
}

func TestSetInvalidValues(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{"output.format", "yaml"},
		{"output.color", "dark"},
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	for _, tt := range tests {
		if err := cfg.Set(tt.key, tt.value); err == nil {
			t.Errorf("Set(%q, %q): expected error, got nil", tt.key, tt.value)
		}
	}
}

func TestDeleteKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	_ = cfg.Set("default.project", "PROJ")
	if err := cfg.Delete("default.project"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := cfg.Get("default.project"); got != "" {
		t.Errorf("expected empty after delete, got %q", got)
	}
}

func TestDeleteInvalidKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if err := cfg.Delete("bogus.key"); err == nil {
		t.Error("expected error for invalid key, got nil")
	}
}

func TestList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Empty config lists nothing.
	if list := cfg.List(); len(list) != 0 {
		t.Errorf("expected empty list, got %v", list)
	}

	_ = cfg.Set("default.project", "PROJ")
	_ = cfg.Set("output.format", "json")
	list := cfg.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(list))
	}
	if list["default.project"] != "PROJ" {
		t.Errorf("default.project = %q, want PROJ", list["default.project"])
	}
	if list["output.format"] != "json" {
		t.Errorf("output.format = %q, want json", list["output.format"])
	}
}

func TestSaveAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	// Create, set, save.
	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	_ = cfg.Set("default.project", "PROJ")
	_ = cfg.Set("output.format", "json")
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload from disk.
	cfg2, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := cfg2.Get("default.project"); got != "PROJ" {
		t.Errorf("after reload: default.project = %q, want PROJ", got)
	}
	if got := cfg2.Get("output.format"); got != "json" {
		t.Errorf("after reload: output.format = %q, want json", got)
	}
}

func TestSaveCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	path := filepath.Join(dir, "config.toml")

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	_ = cfg.Set("default.project", "PROJ")
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}

func TestAtomicWriteNoCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	_ = cfg.Set("default.project", "BEFORE")
	if err := cfg.Save(); err != nil {
		t.Fatalf("first save: %v", err)
	}

	// Overwrite with new value.
	_ = cfg.Set("default.project", "AFTER")
	if err := cfg.Save(); err != nil {
		t.Fatalf("second save: %v", err)
	}

	// No temp files left behind.
	dir := filepath.Dir(path)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "config.toml" {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

// --- Profile tests ---

func TestProfileCRUD(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	fc := cfg.(*fileConfig)

	// No profiles initially.
	if profiles := fc.ListProfiles(); len(profiles) != 0 {
		t.Errorf("expected no profiles, got %v", profiles)
	}

	// Create a profile.
	fc.SetProfile("work", "work.atlassian.net", "alice@work.com")
	p := fc.GetProfile("work")
	if p == nil {
		t.Fatal("GetProfile(work) returned nil")
	}
	if p.Instance != "work.atlassian.net" || p.User != "alice@work.com" {
		t.Errorf("profile mismatch: %+v", p)
	}

	// Unknown profile returns nil.
	if p := fc.GetProfile("nonexistent"); p != nil {
		t.Errorf("expected nil for unknown profile, got %+v", p)
	}
}

func TestActiveProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	fc := cfg.(*fileConfig)

	// Default active profile.
	if got := fc.ActiveProfile(); got != "default" {
		t.Errorf("ActiveProfile() = %q, want \"default\"", got)
	}

	// Set active profile requires it exists.
	fc.SetProfile("work", "work.atlassian.net", "alice@work.com")
	if err := fc.SetActiveProfile("work"); err != nil {
		t.Fatalf("SetActiveProfile(work): %v", err)
	}
	if got := fc.ActiveProfile(); got != "work" {
		t.Errorf("ActiveProfile() = %q, want \"work\"", got)
	}

	// Set to nonexistent profile fails.
	if err := fc.SetActiveProfile("nope"); err == nil {
		t.Error("expected error for nonexistent profile, got nil")
	}
}

func TestDeleteProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	fc := cfg.(*fileConfig)

	fc.SetProfile("work", "work.atlassian.net", "alice@work.com")
	_ = fc.SetActiveProfile("work")

	// Delete active profile clears active.
	if err := fc.DeleteProfile("work"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	if got := fc.ActiveProfile(); got != "default" {
		t.Errorf("after delete, ActiveProfile() = %q, want \"default\"", got)
	}
	if p := fc.GetProfile("work"); p != nil {
		t.Error("deleted profile still exists")
	}

	// Delete nonexistent fails.
	if err := fc.DeleteProfile("work"); err == nil {
		t.Error("expected error for nonexistent profile, got nil")
	}
}

func TestProfileSaveReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	fc := cfg.(*fileConfig)

	fc.SetProfile("work", "work.atlassian.net", "alice@work.com")
	fc.SetProfile("personal", "me.atlassian.net", "bob@me.com")
	_ = fc.SetActiveProfile("work")
	if err := fc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload.
	cfg2, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	fc2 := cfg2.(*fileConfig)

	if got := fc2.ActiveProfile(); got != "work" {
		t.Errorf("after reload: ActiveProfile() = %q, want \"work\"", got)
	}
	profiles := fc2.ListProfiles()
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
	if profiles[0] != "personal" || profiles[1] != "work" {
		t.Errorf("profiles = %v, want [personal work]", profiles)
	}
	p := fc2.GetProfile("work")
	if p == nil || p.Instance != "work.atlassian.net" {
		t.Errorf("reloaded work profile: %+v", p)
	}
}

func TestListProfilesSorted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	fc := cfg.(*fileConfig)

	fc.SetProfile("zebra", "z.atlassian.net", "z@z.com")
	fc.SetProfile("alpha", "a.atlassian.net", "a@a.com")
	fc.SetProfile("mid", "m.atlassian.net", "m@m.com")

	names := fc.ListProfiles()
	if len(names) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(names))
	}
	if names[0] != "alpha" || names[1] != "mid" || names[2] != "zebra" {
		t.Errorf("profiles not sorted: %v", names)
	}
}

// --- Alias tests ---

func TestAliasCRUD(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	fc := cfg.(*fileConfig)

	fc.SetAlias("mine", "issue list --assignee @me")
	aliases := fc.Aliases()
	if aliases["mine"] != "issue list --assignee @me" {
		t.Errorf("alias mine = %q", aliases["mine"])
	}

	fc.DeleteAlias("mine")
	aliases = fc.Aliases()
	if _, ok := aliases["mine"]; ok {
		t.Error("alias mine still exists after delete")
	}
}

func TestAliasSaveReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	fc := cfg.(*fileConfig)

	fc.SetAlias("mine", "issue list --assignee @me")
	if err := fc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	cfg2, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	fc2 := cfg2.(*fileConfig)

	if fc2.Aliases()["mine"] != "issue list --assignee @me" {
		t.Errorf("alias not persisted: %v", fc2.Aliases())
	}
}

func TestGetUnknownKeyReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cfg.Get("nonexistent.key"); got != "" {
		t.Errorf("Get(nonexistent.key) = %q, want empty", got)
	}
}

func TestEmptyValueAllowed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Setting empty string for format/color is valid (clears the value).
	if err := cfg.Set("output.format", ""); err != nil {
		t.Errorf("Set(output.format, \"\") should succeed: %v", err)
	}
	if err := cfg.Set("output.color", ""); err != nil {
		t.Errorf("Set(output.color, \"\") should succeed: %v", err)
	}
}
