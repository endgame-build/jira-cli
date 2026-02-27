// Package shared provides utilities reused across multiple commands.
package shared

import (
	"fmt"
	"io"
	"os"

	"github.com/mattn/go-isatty"

	clierrors "github.com/endgame-build/jira-cli/internal/errors"
)

// MaxBodySize is the maximum allowed size for body content (10 MB).
const MaxBodySize = 10 * 1024 * 1024

// ReadBodyFile reads body content from a file path or stdin ("-").
// Returns the content as a string. Validates:
//   - File exists (VALIDATION_ERROR if not found)
//   - Stdin is not a TTY when "-" is used (VALIDATION_ERROR)
//   - Content does not exceed MaxBodySize (VALIDATION_ERROR)
func ReadBodyFile(path string, stdin io.Reader) (string, error) {
	if path == "-" {
		return readStdin(stdin)
	}
	return readFile(path)
}

// ValidateBodySize checks that body content does not exceed MaxBodySize.
// Use for --description content that bypasses ReadBodyFile.
func ValidateBodySize(content string) error {
	if len(content) > MaxBodySize {
		return clierrors.NewValidationError(
			fmt.Sprintf("Body content exceeds maximum size (%d MB)", MaxBodySize/(1024*1024)),
		).WithSuggestion("Reduce the content size or split into multiple updates")
	}
	return nil
}

func readStdin(stdin io.Reader) (string, error) {
	// Reject if stdin is a TTY (user forgot to pipe).
	if f, ok := stdin.(*os.File); ok {
		if isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd()) {
			return "", clierrors.NewValidationError(
				"--body-file - requires piped input, but stdin is a terminal",
			).WithSuggestion("Pipe content: echo 'description' | jira issue create --body-file -")
		}
	}

	// Read with size limit.
	limited := io.LimitReader(stdin, MaxBodySize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", clierrors.NewValidationError(
			fmt.Sprintf("Failed to read stdin: %v", err),
		)
	}

	if len(data) > MaxBodySize {
		return "", clierrors.NewValidationError(
			fmt.Sprintf("Stdin content exceeds maximum size (%d MB)", MaxBodySize/(1024*1024)),
		).WithSuggestion("Reduce the content size or use a file instead")
	}

	return string(data), nil
}

func readFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", clierrors.NewValidationError(
				fmt.Sprintf("File not found: %s", path),
			).WithSuggestion("Check the file path and try again")
		}
		return "", clierrors.NewValidationError(
			fmt.Sprintf("Cannot access file: %v", err),
		)
	}

	if info.Size() > MaxBodySize {
		return "", clierrors.NewValidationError(
			fmt.Sprintf("File exceeds maximum size (%d MB): %s", MaxBodySize/(1024*1024), path),
		).WithSuggestion("Reduce the file size or split into multiple updates")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", clierrors.NewValidationError(
			fmt.Sprintf("Failed to read file: %v", err),
		)
	}

	return string(data), nil
}
