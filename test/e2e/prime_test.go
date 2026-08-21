//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

// TestE2E_PRIME_01 — `agent prime` emits the documented markdown anchors and
// interpolates the real project key.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-prime-01
func TestE2E_PRIME_01(t *testing.T) {
	h := New(t)

	res := h.MustRun("agent", "prime", "-p", h.Project)
	out := res.Stdout

	for _, anchor := range []string{
		"# Jira Agent Workflow Context",
		"## Rules",
		"## Core Commands",
		"## Session Protocol",
		"## Project: " + h.Project,
	} {
		if !strings.Contains(out, anchor) {
			t.Errorf("prime output is missing the anchor %q\n%s", anchor, res)
		}
	}
	if strings.Contains(out, "## Extended Reference") {
		t.Errorf("prime emitted the extended reference without --full\n%s", res)
	}
	if !strings.Contains(out, "jira agent ready --project "+h.Project) {
		t.Errorf("prime did not interpolate the project key into its commands\n%s", res)
	}
	// A healthy sandbox resolves statuses, types and the sprint without warnings.
	RequireNoStderr(t, res)
}

// TestE2E_PRIME_02 — --full adds exactly one section and the default output is
// a strict prefix of it.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-prime-02
func TestE2E_PRIME_02(t *testing.T) {
	h := New(t)

	plain := h.MustRun("agent", "prime", "-p", h.Project).Stdout
	full := h.MustRun("agent", "prime", "-p", h.Project, "--full").Stdout

	if !strings.HasPrefix(full, plain) {
		t.Errorf("--full output is not an extension of the default output")
	}
	for _, anchor := range []string{
		"## Extended Reference",
		"### Ready Queue Flags",
		"### Discover Flags",
		"### Close Flags",
	} {
		if !strings.Contains(full, anchor) {
			t.Errorf("--full output is missing %q", anchor)
		}
	}
}

// TestE2E_PRIME_03 — with an active sprint, prime carries a sprint section and
// its Session Protocol step 1 includes --sprint active.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-prime-03
func TestE2E_PRIME_03(t *testing.T) {
	h := New(t)

	out := h.MustRun("agent", "prime", "-p", h.Project).Stdout

	if !strings.Contains(out, "## Sprint: "+h.Sandbox.Sprint.Name) {
		t.Errorf("prime is missing the sprint section for %q\n%s", h.Sandbox.Sprint.Name, out)
	}
	want := "1. `jira agent ready --project " + h.Project + " --sprint active --json`"
	if !strings.Contains(out, want) {
		t.Errorf("Session Protocol step 1 does not steer to the active sprint; want %q\n%s", want, out)
	}
}

// TestE2E_PRIME_04 — prime ignores --json, --jq, --quiet and --dry-run and
// always emits markdown. An agent hook that assumes --json everywhere gets
// markdown here, and --quiet cannot silence it.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-prime-04
func TestE2E_PRIME_04(t *testing.T) {
	h := New(t)

	base := h.MustRun("agent", "prime", "-p", h.Project).Stdout

	for _, extra := range [][]string{
		{"--json"},
		{"--jq", "."},
		{"--quiet"},
		{"--dry-run"},
	} {
		args := append([]string{"agent", "prime", "-p", h.Project}, extra...)
		res := h.MustRun(args...)
		if res.Stdout != base {
			t.Errorf("prime %v changed its output; it is documented to ignore these flags\n%s", extra, res)
		}
	}
}

// TestE2E_PRIME_05 — prime reports the project's real statuses and types, not a
// generic list. This is also the cross-check for the hardcoded sub-task type in
// agent discover: if prime prints "Subtask" here, the default discover fails.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-prime-05
func TestE2E_PRIME_05(t *testing.T) {
	h := New(t)

	out := h.MustRun("agent", "prime", "-p", h.Project).Stdout

	if strings.Contains(out, "unable to fetch") {
		t.Errorf("prime could not resolve the project metadata\n%s", out)
	}
	if !strings.Contains(out, "- **Statuses:**") || !strings.Contains(out, "- **Types:**") {
		t.Fatalf("prime is missing its project metadata lines\n%s", out)
	}
	for _, category := range []string{"(new)", "(indeterminate)", "(done)"} {
		if !strings.Contains(out, category) {
			t.Errorf("prime lists no status in the %s category; claim or close will have no "+
				"transition to resolve\n%s", category, out)
		}
	}
	if !strings.Contains(out, h.Sandbox.SubtaskType+" (subtask)") {
		t.Errorf("prime does not list the sub-task type %q\n%s", h.Sandbox.SubtaskType, out)
	}
}
