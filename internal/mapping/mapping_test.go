package mapping

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testConfig = `
project: LMP
issue_types:
  epic: Epic
  story: Task
links:
  initiative: { via: parent }
  epic: { via: parent }
priority_map:
  High: "High (migrated)"
  Critical: "Critical (migrated)"
streams:
  EP-TECH: { stream_label: "stream:tech" }
  EP-LMP: { stream_label: "stream:lmp" }
`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func loadTestConfig(t *testing.T) *Config {
	t.Helper()
	dir := t.TempDir()
	p := writeFile(t, dir, "jira-sync.yaml", testConfig)
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return cfg
}

func TestLoadConfig_Validations(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadConfig(filepath.Join(dir, "nope.yaml")); err == nil {
		t.Error("expected error for missing file")
	}
	p := writeFile(t, dir, "no-project.yaml", "issue_types:\n  epic: Epic\n  story: Task\n")
	if _, err := LoadConfig(p); err == nil {
		t.Error("expected error for missing project")
	}
	p = writeFile(t, dir, "no-types.yaml", "project: LMP\n")
	if _, err := LoadConfig(p); err == nil {
		t.Error("expected error for missing issue_types")
	}
}

func TestParseMappedFile_EpicCreate(t *testing.T) {
	cfg := loadTestConfig(t)
	dir := t.TempDir()
	p := writeFile(t, dir, "EP-TECH-01.md", `---
id: EP-TECH-01
name: "Bi-directional JIRA sync"
status: "Planned"
priority: "High"
initiative: "TI-886"
stream: "technical"
jira_key: null
jira_project: "LMP"
jira_issue_type: "Epic"
assignee: null
last_synced_at: null
updated: 2026-07-12
---
# EP-TECH-01
Body text.`)

	n := 0
	f, err := ParseMappedFile(p, cfg, &n)
	if err != nil {
		t.Fatal(err)
	}
	fm := f.Frontmatter
	if !f.IsCreate() {
		t.Errorf("expected create, key=%q", fm.Key)
	}
	if fm.Key != "LMP-NEW-1" {
		t.Errorf("Key = %q, want LMP-NEW-1", fm.Key)
	}
	if fm.Summary != "Bi-directional JIRA sync" {
		t.Errorf("Summary = %q", fm.Summary)
	}
	if fm.Type != "Epic" {
		t.Errorf("Type = %q, want Epic", fm.Type)
	}
	if fm.Project != "LMP" {
		t.Errorf("Project = %q", fm.Project)
	}
	if fm.Parent != "TI-886" {
		t.Errorf("Parent = %q, want TI-886 (from initiative)", fm.Parent)
	}
	if fm.Priority != "High (migrated)" {
		t.Errorf("Priority = %q, want mapped 'High (migrated)'", fm.Priority)
	}
	if len(fm.Labels) != 1 || fm.Labels[0] != "stream:tech" {
		t.Errorf("Labels = %v, want [stream:tech]", fm.Labels)
	}
	if !strings.Contains(f.Description, "Body text.") {
		t.Errorf("Description missing body: %q", f.Description)
	}
	if fm.Status != "" {
		t.Errorf("Status should not be pushed, got %q", fm.Status)
	}
}

func TestParseMappedFile_StoryCreate(t *testing.T) {
	cfg := loadTestConfig(t)
	dir := t.TempDir()
	p := writeFile(t, dir, "EP-TECH-01-01.md", `---
id: EP-TECH-01-01
name: "Story one"
parent_epic_id: EP-TECH-01
parent_epic_jira_key: "LMP-59"
jira_key: null
jira_project: "LMP"
jira_issue_type: "Task"
last_synced_at: null
updated: 2026-07-12
---
Story body.`)

	n := 0
	f, err := ParseMappedFile(p, cfg, &n)
	if err != nil {
		t.Fatal(err)
	}
	fm := f.Frontmatter
	if fm.Type != "Task" {
		t.Errorf("Type = %q, want Task", fm.Type)
	}
	if fm.Parent != "LMP-59" {
		t.Errorf("Parent = %q, want LMP-59 (from parent_epic_jira_key)", fm.Parent)
	}
	if len(fm.Labels) != 1 || fm.Labels[0] != "stream:tech" {
		t.Errorf("Labels = %v", fm.Labels)
	}
}

func TestParseMappedFile_Update(t *testing.T) {
	cfg := loadTestConfig(t)
	dir := t.TempDir()
	p := writeFile(t, dir, "EP-TECH-01.md", `---
id: EP-TECH-01
name: "Existing epic"
initiative: "TI-886"
jira_key: "LMP-59"
jira_project: "LMP"
jira_issue_type: "Epic"
last_synced_at: "2026-07-12T07:11:31Z"
updated: 2026-07-12
---
Body.`)

	n := 0
	f, err := ParseMappedFile(p, cfg, &n)
	if err != nil {
		t.Fatal(err)
	}
	if f.IsCreate() {
		t.Error("expected update for real jira_key")
	}
	if f.Frontmatter.Key != "LMP-59" {
		t.Errorf("Key = %q, want LMP-59", f.Frontmatter.Key)
	}
	if f.Frontmatter.Updated != "2026-07-12T07:11:31Z" {
		t.Errorf("Updated = %q, want last_synced_at value (conflict anchor)", f.Frontmatter.Updated)
	}
	if n != 0 {
		t.Errorf("tempCounter should not increment for update, got %d", n)
	}
}

func TestParseMappedFile_MissingName(t *testing.T) {
	cfg := loadTestConfig(t)
	dir := t.TempDir()
	p := writeFile(t, dir, "bad.md", "---\nid: EP-X-01\njira_project: LMP\n---\nbody")
	n := 0
	if _, err := ParseMappedFile(p, cfg, &n); err == nil {
		t.Error("expected error for missing name")
	}
}

func TestWriteBack(t *testing.T) {
	dir := t.TempDir()
	orig := `---
id: EP-TECH-01
name: "Epic"
initiative: "TI-886"
jira_key: null                          # set on first push
jira_project: "LMP"
last_synced_at: null
updated: 2026-07-12
---
Body stays intact.`
	p := writeFile(t, dir, "EP-TECH-01.md", orig)

	if err := WriteBack(p, "LMP-99", "2026-07-12T08:00:00Z"); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(p)
	s := string(out)
	if !strings.Contains(s, `jira_key: "LMP-99"`) {
		t.Errorf("jira_key not written: \n%s", s)
	}
	if !strings.Contains(s, `last_synced_at: "2026-07-12T08:00:00Z"`) {
		t.Errorf("last_synced_at not written:\n%s", s)
	}
	if !strings.Contains(s, `# set on first push`) {
		t.Errorf("trailing comment on jira_key line should be preserved:\n%s", s)
	}
	if !strings.Contains(s, `initiative: "TI-886"`) || !strings.Contains(s, "Body stays intact.") {
		t.Errorf("other content must be preserved:\n%s", s)
	}
	// Idempotent re-parse: now an update with the written key.
	cfg := loadTestConfig(t)
	n := 0
	f, err := ParseMappedFile(p, cfg, &n)
	if err != nil {
		t.Fatal(err)
	}
	if f.IsCreate() || f.Frontmatter.Key != "LMP-99" {
		t.Errorf("after write-back, expected update with key LMP-99, got create=%v key=%q", f.IsCreate(), f.Frontmatter.Key)
	}
}

func TestWriteBack_InsertsMissingKeys(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "min.md", "---\nid: EP-X-01\nname: X\n---\nbody")
	if err := WriteBack(p, "LMP-1", "2026-07-12T08:00:00Z"); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(p)
	s := string(out)
	if !strings.Contains(s, `jira_key: "LMP-1"`) || !strings.Contains(s, `last_synced_at: "2026-07-12T08:00:00Z"`) {
		t.Errorf("missing keys not inserted:\n%s", s)
	}
	if !strings.Contains(s, "body") {
		t.Errorf("body lost:\n%s", s)
	}
}
