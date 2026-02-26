package shared

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	clierrors "github.com/endgameio/jira-cli/internal/errors"
)

func TestReadBodyFile_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "desc.md")
	content := "# Hello\n\nSome **bold** text."
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadBodyFile(path, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != content {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestReadBodyFile_FileNotFound(t *testing.T) {
	_, err := ReadBodyFile("/nonexistent/file.md", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.VALIDATION_ERROR)
	}
	if !strings.Contains(cliErr.Message, "File not found") {
		t.Errorf("error message = %q, want 'File not found'", cliErr.Message)
	}
}

func TestReadBodyFile_FromStdin(t *testing.T) {
	content := "Description from stdin"
	stdin := bytes.NewBufferString(content)

	got, err := ReadBodyFile("-", stdin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != content {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestReadBodyFile_StdinTooLarge(t *testing.T) {
	// Create content exceeding MaxBodySize.
	bigContent := strings.Repeat("x", MaxBodySize+1)
	stdin := bytes.NewBufferString(bigContent)

	_, err := ReadBodyFile("-", stdin)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.VALIDATION_ERROR)
	}
	if !strings.Contains(cliErr.Message, "exceeds maximum size") {
		t.Errorf("error message = %q, want size error", cliErr.Message)
	}
}

func TestReadBodyFile_FileTooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.md")

	// Create a file just over the limit.
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	// Write MaxBodySize+1 bytes.
	if _, err := f.Write(make([]byte, MaxBodySize+1)); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	_, err = ReadBodyFile(path, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.VALIDATION_ERROR)
	}
}

func TestValidateBodySize_OK(t *testing.T) {
	if err := ValidateBodySize("short content"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBodySize_TooLarge(t *testing.T) {
	big := strings.Repeat("x", MaxBodySize+1)
	err := ValidateBodySize(big)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.VALIDATION_ERROR)
	}
}
