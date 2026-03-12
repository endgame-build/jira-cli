package markdown

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FieldValueMap stores raw API objects for custom field values that need
// object wrapping during import. Structure:
//
//	normalizedFieldName → displayValue → rawJSON
//
// Only populated for fields where the API returns an object (e.g. team,
// option, user). Scalar values (string, number, bool) don't need this.
type FieldValueMap map[string]map[string]json.RawMessage

// FieldValuesFileName is the conventional name for the sidecar file.
const FieldValuesFileName = ".jira-field-values.json"

// LoadFieldValues reads a FieldValueMap from the given path.
// Returns an empty map (not nil) if the file does not exist.
func LoadFieldValues(path string) (FieldValueMap, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(FieldValueMap), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read field values: %w", err)
	}

	var fvm FieldValueMap
	if err := json.Unmarshal(data, &fvm); err != nil {
		return nil, fmt.Errorf("parse field values: %w", err)
	}
	if fvm == nil {
		fvm = make(FieldValueMap)
	}
	return fvm, nil
}

// SaveFieldValues writes a FieldValueMap to the given path atomically.
func SaveFieldValues(path string, fvm FieldValueMap) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory for field values: %w", err)
	}

	data, err := json.MarshalIndent(fvm, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal field values: %w", err)
	}
	data = append(data, '\n')

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write field values: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename field values: %w", err)
	}
	return nil
}

// Merge adds entries from other into fvm. Existing entries are preserved
// (first-write wins), so earlier exports don't lose values.
func (fvm FieldValueMap) Merge(other FieldValueMap) {
	for field, vals := range other {
		if fvm[field] == nil {
			fvm[field] = make(map[string]json.RawMessage)
		}
		for display, raw := range vals {
			if _, exists := fvm[field][display]; !exists {
				fvm[field][display] = raw
			}
		}
	}
}

// FindFieldValues searches for a .jira-field-values.json file starting from
// dir, then checking the parent directory (one level up). Returns the loaded
// map and the path found, or an empty map if not found.
func FindFieldValues(dir string) (FieldValueMap, string, error) {
	for _, d := range []string{dir, filepath.Dir(dir)} {
		path := filepath.Join(d, FieldValuesFileName)
		if _, err := os.Stat(path); err == nil {
			fvm, err := LoadFieldValues(path)
			return fvm, path, err
		}
	}
	return make(FieldValueMap), "", nil
}
