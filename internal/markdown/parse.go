package markdown

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	clierrors "github.com/endgame-build/jira-cli/internal/errors"

	"gopkg.in/yaml.v3"
)

// IssueFile represents a parsed markdown file with YAML frontmatter.
type IssueFile struct {
	Path        string
	Frontmatter Frontmatter
	Description string // raw markdown body after frontmatter
}

// tempKeyPattern matches temporary issue keys like PROJ-NEW-1, PROJ-NEW-42.
var tempKeyPattern = regexp.MustCompile(`^[A-Z]+-NEW-\d+$`)

// IsCreate returns true if the issue key matches the temp key pattern (e.g. PROJ-NEW-1).
func (f *IssueFile) IsCreate() bool {
	return tempKeyPattern.MatchString(f.Frontmatter.Key)
}

// IsTempKey returns true if the given key matches the temp key pattern.
func IsTempKey(key string) bool {
	return tempKeyPattern.MatchString(key)
}

// ParseFile reads a markdown file, splits YAML frontmatter from body, and validates.
// Returns CLIError with VALIDATION_ERROR if frontmatter is missing or invalid.
func ParseFile(path string) (*IssueFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, clierrors.NewValidationError("failed to read file: " + path).WithErr(err)
	}

	content := string(data)

	// Find frontmatter delimiters
	if !strings.HasPrefix(content, "---\n") {
		return nil, clierrors.NewValidationError("missing YAML frontmatter delimiters").
			WithContext(map[string]interface{}{"path": path}).
			WithSuggestion("file must start with --- followed by YAML frontmatter and closing ---")
	}

	// Find closing delimiter (skip the opening "---\n")
	closeIdx := strings.Index(content[4:], "\n---\n")
	if closeIdx < 0 {
		// Check for closing delimiter at end of file (no trailing newline after ---)
		closeIdx = strings.Index(content[4:], "\n---")
		if closeIdx < 0 || closeIdx+4+4 != len(content) {
			return nil, clierrors.NewValidationError("missing closing YAML frontmatter delimiter").
				WithContext(map[string]interface{}{"path": path}).
				WithSuggestion("frontmatter must be enclosed between --- delimiters")
		}
	}

	yamlContent := content[4 : 4+closeIdx]

	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return nil, clierrors.NewValidationError("failed to parse YAML frontmatter").
			WithContext(map[string]interface{}{"path": path}).
			WithErr(err)
	}

	// Second pass: capture unknown keys as custom fields.
	var rawMap map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &rawMap); err != nil {
		return nil, clierrors.NewValidationError("failed to parse custom fields from frontmatter").
			WithContext(map[string]interface{}{"path": path}).
			WithErr(err)
	}
	for k, v := range rawMap {
		if !IsBuiltinKey(k) {
			if fm.CustomFields == nil {
				fm.CustomFields = make(map[string]interface{})
			}
			fm.CustomFields[k] = v
		}
	}

	if fm.Key == "" {
		return nil, clierrors.NewValidationError("missing required field: key").
			WithContext(map[string]interface{}{"path": path}).
			WithSuggestion("add 'key: PROJ-123' to the YAML frontmatter")
	}

	// Body is everything after the closing delimiter, trimmed
	bodyStart := 4 + closeIdx + 4 // skip opening "---\n" + yaml + "\n---\n"
	body := ""
	if bodyStart < len(content) {
		body = strings.TrimSpace(content[bodyStart:])
	}

	return &IssueFile{
		Path:        path,
		Frontmatter: fm,
		Description: body,
	}, nil
}

// ParseDir recursively finds all .md files in dir and parses them, sorted by path.
func ParseDir(dir string) ([]*IssueFile, error) {
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

	var files []*IssueFile
	for _, p := range paths {
		f, err := ParseFile(p)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}

	return files, nil
}
