package mapping

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/markdown"

	"gopkg.in/yaml.v3"
)

// ParseMappedDir recursively parses and maps all .md files in dir, sorted by path.
func ParseMappedDir(dir string, cfg *Config) ([]*markdown.IssueFile, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".md") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, clierrors.NewValidationError("failed to walk directory: " + dir).WithErr(err)
	}
	sort.Strings(paths)

	tempCounter := 0
	var files []*markdown.IssueFile
	for _, p := range paths {
		f, err := ParseMappedFile(p, cfg, &tempCounter)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

// ParseMappedFiles parses and maps an explicit list of files.
func ParseMappedFiles(paths []string, cfg *Config) ([]*markdown.IssueFile, error) {
	tempCounter := 0
	var files []*markdown.IssueFile
	for _, p := range paths {
		f, err := ParseMappedFile(p, cfg, &tempCounter)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

// ParseMappedFile reads a hub-style markdown file and maps its frontmatter into
// the canonical markdown.Frontmatter the import pipeline expects. tempCounter is
// incremented to mint a unique temp key (PROJECT-NEW-N) for documents without a
// jira_key (which therefore create rather than update).
func ParseMappedFile(path string, cfg *Config, tempCounter *int) (*markdown.IssueFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, clierrors.NewValidationError("failed to read file: " + path).WithErr(err)
	}
	yamlContent, body, err := markdown.SplitFrontmatter(string(data), path)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &raw); err != nil {
		return nil, clierrors.NewValidationError("failed to parse YAML frontmatter").
			WithContext(map[string]interface{}{"path": path}).WithErr(err)
	}

	id := str(raw, "id")
	name := str(raw, "name")
	if name == "" {
		return nil, clierrors.NewValidationError("mapped file missing 'name': " + path).
			WithSuggestion("hub epics/stories must carry a 'name:' used as the JIRA summary")
	}

	isStory := str(raw, "parent_epic_id") != "" || str(raw, "parent_epic_jira_key") != "" ||
		str(raw, "jira_issue_type") == cfg.IssueTypes.Story

	fm := markdown.Frontmatter{
		ID:      id,
		Summary: name,
		Project: firstNonEmpty(str(raw, "jira_project"), cfg.Project),
		Updated: str(raw, "last_synced_at"), // conflict anchor: last push time
	}

	// Issue type: explicit jira_issue_type wins, else derive from story/epic.
	if t := str(raw, "jira_issue_type"); t != "" {
		fm.Type = t
	} else if isStory {
		fm.Type = cfg.IssueTypes.Story
	} else {
		fm.Type = cfg.IssueTypes.Epic
	}

	// Key: real jira_key updates; absent → temp key creates.
	if k := str(raw, "jira_key"); k != "" {
		fm.Key = k
	} else {
		*tempCounter++
		fm.Key = fmt.Sprintf("%s-NEW-%d", fm.Project, *tempCounter)
	}

	// Parent: story → parent epic key; epic → initiative. `links` declares the
	// mechanism; v1 supports the native `parent` field only.
	if err := applyParent(&fm, raw, cfg, isStory, path); err != nil {
		return nil, err
	}

	// Priority: map hub priority → JIRA (migrated) priority name.
	if p := str(raw, "priority"); p != "" {
		if mapped, ok := cfg.PriorityMap[p]; ok {
			fm.Priority = mapped
		} else {
			fm.Priority = p // pass through; let JIRA validate
		}
	}

	// Labels: the lightweight stream tag, derived from the id prefix (EP-LMP-… → EP-LMP).
	if label := streamLabel(id, cfg); label != "" {
		fm.Labels = []string{label}
	}

	// status + assignee are JIRA-first (pull-only) and deliberately never pushed.

	return &markdown.IssueFile{Path: path, Frontmatter: fm, Description: body}, nil
}

// applyParent resolves and sets the parent per the links mechanism.
func applyParent(fm *markdown.Frontmatter, raw map[string]interface{}, cfg *Config, isStory bool, path string) error {
	var target string
	var linkName string
	if isStory {
		target, linkName = str(raw, "parent_epic_jira_key"), "epic"
	} else {
		target, linkName = str(raw, "initiative"), "initiative"
	}
	if target == "" {
		return nil // no parent to attach (e.g. an unparented epic, or a story before its epic exists)
	}
	via := "parent"
	if link, ok := cfg.Links[linkName]; ok && link.Via != "" {
		via = link.Via
	}
	if via != "parent" {
		return clierrors.NewValidationError(
			fmt.Sprintf("links.%s.via=%q not supported yet (only 'parent'): %s", linkName, via, path),
		).WithSuggestion("v1 attaches parents via the native 'parent' field")
	}
	fm.Parent = target
	return nil
}

var streamKeyRe = regexp.MustCompile(`^([A-Z]+-[A-Z]+)`)

// streamLabel derives the JIRA stream label from a hub id prefix (EP-LMP-00 → EP-LMP).
func streamLabel(id string, cfg *Config) string {
	m := streamKeyRe.FindStringSubmatch(id)
	if m == nil {
		return ""
	}
	if s, ok := cfg.Streams[m[1]]; ok {
		return s.Label
	}
	return ""
}

var (
	jiraKeyLineRe    = regexp.MustCompile(`(?m)^(jira_key:)[^\n]*$`)
	lastSyncedLineRe = regexp.MustCompile(`(?m)^(last_synced_at:)[^\n]*$`)
)

// WriteBack rewrites a mapped file's frontmatter after a push: it sets jira_key
// (for a fresh create) and last_synced_at, preserving everything else. Only the
// frontmatter region (before the closing ---) is touched. If a key is absent it
// is inserted before the closing delimiter.
func WriteBack(path, key, syncedAt string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	// Bound edits to the frontmatter: split at the closing delimiter.
	closeRel := strings.Index(content[4:], "\n---")
	if !strings.HasPrefix(content, "---\n") || closeRel < 0 {
		return fmt.Errorf("write-back: no frontmatter in %s", path)
	}
	fmEnd := 4 + closeRel // index of the "\n---" that closes frontmatter
	head, tail := content[:fmEnd], content[fmEnd:]

	head = setFrontmatterLine(head, jiraKeyLineRe, "jira_key", key)
	head = setFrontmatterLine(head, lastSyncedLineRe, "last_synced_at", syncedAt)

	return os.WriteFile(path, []byte(head+tail), 0o644)
}

// setFrontmatterLine replaces an existing `field:` line (preserving any trailing
// # comment) or appends the line if absent.
func setFrontmatterLine(head string, re *regexp.Regexp, field, value string) string {
	repl := fmt.Sprintf("%s: %q", field, value)
	if loc := re.FindStringIndex(head); loc != nil {
		line := head[loc[0]:loc[1]]
		if i := strings.Index(line, "#"); i >= 0 { // keep trailing comment
			repl += "  " + strings.TrimRight(line[i:], " ")
		}
		return head[:loc[0]] + repl + head[loc[1]:]
	}
	return strings.TrimRight(head, "\n") + "\n" + repl
}

func str(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok && v != nil {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
