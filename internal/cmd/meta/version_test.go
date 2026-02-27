package meta

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/config"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/iostreams"
)

// buildVersionTestRoot constructs a minimal root with the meta version command.
func buildVersionTestRoot(f *factory.Factory) *cobra.Command {
	root := &cobra.Command{
		Use:     "jira <command> [flags]",
		Short:   "Jira CLI",
		Version: "0.1.0",
	}

	metaCmd := &cobra.Command{
		Use:   "meta <command>",
		Short: "CLI introspection",
	}
	metaCmd.AddCommand(NewCmdVersion(f))
	root.AddCommand(metaCmd)

	return root
}

func TestMetaVersion_JSONDefault(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	// Clear env vars to ensure instance is null.
	t.Setenv("JIRA_INSTANCE", "")

	root := buildVersionTestRoot(f)
	root.SetArgs([]string{"meta", "version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()

	var info VersionInfo
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		t.Fatalf("output should be valid JSON: %v\nOutput: %s", err, out)
	}

	if info.Version != "0.1.0" {
		t.Errorf("version = %q, want %q", info.Version, "0.1.0")
	}
	if info.API != "jira-cloud-v3" {
		t.Errorf("api = %q, want %q", info.API, "jira-cloud-v3")
	}
	if info.Instance != nil {
		t.Errorf("instance = %v, want null", *info.Instance)
	}
}

func TestMetaVersion_InstanceFromFlag(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)
	f.FlagInstance = "mycompany.atlassian.net"

	// Clear env to ensure flag takes precedence.
	t.Setenv("JIRA_INSTANCE", "")

	root := buildVersionTestRoot(f)
	root.SetArgs([]string{"meta", "version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var info VersionInfo
	if err := json.Unmarshal(tio.OutBuf.Bytes(), &info); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if info.Instance == nil {
		t.Fatal("expected instance to be populated")
	}
	if *info.Instance != "mycompany.atlassian.net" {
		t.Errorf("instance = %q, want %q", *info.Instance, "mycompany.atlassian.net")
	}
}

func TestMetaVersion_InstanceFromEnv(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	t.Setenv("JIRA_INSTANCE", "env-site.atlassian.net")

	root := buildVersionTestRoot(f)
	root.SetArgs([]string{"meta", "version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var info VersionInfo
	if err := json.Unmarshal(tio.OutBuf.Bytes(), &info); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if info.Instance == nil {
		t.Fatal("expected instance to be populated")
	}
	if *info.Instance != "env-site.atlassian.net" {
		t.Errorf("instance = %q, want %q", *info.Instance, "env-site.atlassian.net")
	}
}

func TestMetaVersion_InstanceFromProfile(t *testing.T) {
	tio := iostreams.Test()

	// Write config with a profile.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	tomlData := `active_profile = "default"

[profiles.default]
instance = "profile-site.atlassian.net"
user = "user@example.com"
`
	if err := os.WriteFile(cfgPath, []byte(tomlData), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	f := factory.NewTestFactory(tio.IOStreams, cfg, nil)

	// Clear env and flags.
	t.Setenv("JIRA_INSTANCE", "")

	root := buildVersionTestRoot(f)
	root.SetArgs([]string{"meta", "version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var info VersionInfo
	if err := json.Unmarshal(tio.OutBuf.Bytes(), &info); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if info.Instance == nil {
		t.Fatal("expected instance to be populated from profile")
	}
	if *info.Instance != "profile-site.atlassian.net" {
		t.Errorf("instance = %q, want %q", *info.Instance, "profile-site.atlassian.net")
	}
}

func TestMetaVersion_TextOutput(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)
	f.Text = true
	f.FlagInstance = "mycompany.atlassian.net"

	// Clear env.
	t.Setenv("JIRA_INSTANCE", "")

	root := buildVersionTestRoot(f)
	root.SetArgs([]string{"meta", "version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()

	if !strings.Contains(out, "0.1.0") {
		t.Error("text output should contain version")
	}
	if !strings.Contains(out, "jira-cloud-v3") {
		t.Error("text output should contain API target")
	}
	if !strings.Contains(out, "mycompany.atlassian.net") {
		t.Error("text output should contain instance")
	}
}

func TestMetaVersion_TextNoInstance(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)
	f.Text = true

	// Clear env.
	t.Setenv("JIRA_INSTANCE", "")

	root := buildVersionTestRoot(f)
	root.SetArgs([]string{"meta", "version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()

	if !strings.Contains(out, "(not configured)") {
		t.Error("text output should show '(not configured)' when no instance is set")
	}
}

func TestMetaVersion_NoAuthTriggered(t *testing.T) {
	// Factory with nil client — if auth is triggered, it would fail.
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	t.Setenv("JIRA_INSTANCE", "")

	root := buildVersionTestRoot(f)
	root.SetArgs([]string{"meta", "version"})

	// This should succeed without trying to resolve auth.
	if err := root.Execute(); err != nil {
		t.Fatalf("meta version should work without auth: %v", err)
	}
}
