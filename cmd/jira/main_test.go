package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary compiles the CLI binary into a temp directory and returns its path.
func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "jira")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = filepath.Join(testProjectRoot(t), "cmd", "jira")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

// testProjectRoot walks up from the test file to find the project root (containing go.mod).
func testProjectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (go.mod)")
		}
		dir = parent
	}
}

func TestBuildBinary(t *testing.T) {
	bin := buildBinary(t)
	info, err := os.Stat(bin)
	if err != nil {
		t.Fatalf("binary not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("binary is empty")
	}
}

func TestVersionFlag(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("--version should exit 0, got error: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "dev") {
		t.Errorf("expected version output to contain 'dev', got: %s", out)
	}
}

func TestBadFlagsExitCode(t *testing.T) {
	bin := buildBinary(t)
	// --json and --text together should trigger VALIDATION_ERROR (exit 3).
	cmd := exec.Command(bin, "--json", "--text", "auth")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for --json --text, got 0")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got: %T: %v", err, err)
	}
	if exitErr.ExitCode() != 3 {
		t.Errorf("expected exit code 3 (VALIDATION_ERROR), got %d\noutput: %s", exitErr.ExitCode(), out)
	}
	// Should contain the error message about conflicting flags.
	if !strings.Contains(string(out), "--json") || !strings.Contains(string(out), "--text") {
		t.Errorf("expected error mentioning --json and --text, got: %s", out)
	}
}

func TestHelpNoError(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("--help should exit 0, got error: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Jira Cloud") {
		t.Errorf("expected help to mention 'Jira Cloud', got: %s", out)
	}
}

func TestRunFunction(t *testing.T) {
	// Test that run() returns 0 for --help (sets os.Args).
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	os.Args = []string{"jira", "--help"}
	code := run()
	if code != 0 {
		t.Errorf("expected exit code 0 for --help, got %d", code)
	}
}
