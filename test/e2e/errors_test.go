//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

// TestE2E_ERR_01 — every documented exit code, asserted against the real CLI.
// The mapping lives in internal/errors/errors.go and is the contract agents
// branch on, so a silent change here breaks every consumer.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-err-01
func TestE2E_ERR_01(t *testing.T) {
	h := New(t)

	cases := []struct {
		id       string
		args     []string
		wantExit int
		wantCode string
	}{
		{
			id:       "E2E-ERR-01a-unknown-issue-key",
			args:     []string{"agent", "claim", h.Project + "-999999", "--json"},
			wantExit: 4,
			wantCode: "NOT_FOUND",
		},
		{
			id:       "E2E-ERR-01b-unknown-issue-on-close",
			args:     []string{"agent", "close", h.Project + "-999999", "--json"},
			wantExit: 4,
			wantCode: "NOT_FOUND",
		},
		{
			id:       "E2E-ERR-01c-malformed-issue-key",
			args:     []string{"agent", "claim", "not-a-key", "--json"},
			wantExit: 3,
			wantCode: "VALIDATION_ERROR",
		},
		{
			id:       "E2E-ERR-01d-mutually-exclusive-assignee-flags",
			args:     []string{"agent", "ready", "-p", h.Project, "--assignee", "@me", "--unassigned", "--json"},
			wantExit: 3,
			wantCode: "VALIDATION_ERROR",
		},
		{
			id:       "E2E-ERR-01e-discover-without-title",
			args:     []string{"agent", "discover", h.Project + "-1", "--json"},
			wantExit: 3,
			wantCode: "VALIDATION_ERROR",
		},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			res := h.RunExpectExit(tc.wantExit, tc.args...)
			RequireEmptyStdout(t, res)
			DecodeError(t, res, tc.wantCode)
		})
	}
}

// TestE2E_ERR_02 — the two flag-validation guards run before any HTTP request.
// Pointing the CLI at an unroutable host proves it: a guard that fired would
// exit 3, and one that did not would exit 7 on the network error.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-err-02
func TestE2E_ERR_02(t *testing.T) {
	h := New(t)

	unreachable := replaceEnv(h.Env(), EnvInstance, "127.0.0.1:1")

	t.Run("E2E-ERR-02a-assignee-conflict-is-pre-network", func(t *testing.T) {
		res := h.RunEnv(unreachable, "agent", "ready", "-p", h.Project, "--assignee", "@me", "--unassigned", "--json")
		if res.ExitCode != 3 {
			t.Errorf("exit = %d, want 3 — the flag guard should fire before any request\n%s", res.ExitCode, res)
		}
	})

	t.Run("E2E-ERR-02b-missing-title-is-pre-network", func(t *testing.T) {
		res := h.RunEnv(unreachable, "agent", "discover", h.Project+"-1", "--json")
		if res.ExitCode != 3 {
			t.Errorf("exit = %d, want 3 — the --title guard should fire before any request\n%s", res.ExitCode, res)
		}
	})
}

// TestE2E_ERR_03 — bad credentials exit 2, and an unresolvable host exits 7.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-err-03
func TestE2E_ERR_03(t *testing.T) {
	h := New(t)

	t.Run("E2E-ERR-03a-bad-token-exits-2", func(t *testing.T) {
		env := replaceEnv(h.Env(), EnvToken, "definitely-not-a-valid-token")
		res := h.RunEnv(env, "agent", "ready", "-p", h.Project, "--json")
		if res.ExitCode != 2 {
			t.Errorf("exit = %d, want 2 (AUTH_ERROR)\n%s", res.ExitCode, res)
		}
	})

	t.Run("E2E-ERR-03b-unresolvable-host-exits-7", func(t *testing.T) {
		env := replaceEnv(h.Env(), EnvInstance, "nonexistent-host-for-e2e.invalid")
		res := h.RunEnv(env, "agent", "ready", "-p", h.Project, "--json")
		if res.ExitCode != 7 {
			t.Errorf("exit = %d, want 7 (NETWORK_ERROR)\n%s", res.ExitCode, res)
		}
	})
}

// TestE2E_ERR_04 — issue keys whose project part contains a digit are rejected
// before any request, even though Jira allows them.
//
// internal/cmd/shared/validate.go:12 is `^[A-Za-z][A-Za-z]*-[0-9]+$`, so a
// project like AB1 makes every claim/close/discover call unusable while
// `agent ready -p AB1` still returns those very keys. The loop hands an agent
// work it then refuses to accept.
//
// This case documents the defect. It passes today by asserting the broken
// behaviour, and will fail — deliberately — when the regex is fixed.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-err-04
func TestE2E_ERR_04(t *testing.T) {
	h := New(t)

	res := h.Run("agent", "claim", "AB1-23", "--json")
	if res.ExitCode != 3 {
		t.Skipf("expected the digit-bearing key AB1-23 to be rejected with exit 3; got %d — "+
			"the validation regex may have been fixed, in which case delete this case", res.ExitCode)
	}
	doc := DecodeError(t, res, "VALIDATION_ERROR")
	if !strings.Contains(strings.ToLower(doc.Message), "invalid issue key") {
		t.Errorf("message = %q, want it to name an invalid issue key\n%s", doc.Message, res)
	}
	t.Logf("KNOWN DEFECT: issue keys with a digit in the project part (AB1-23) are rejected "+
		"pre-request by internal/cmd/shared/validate.go:12, though Jira permits them: %s", doc.Message)
}

// replaceEnv returns env with key set to value, replacing any existing entry.
func replaceEnv(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return append(out, key+"="+value)
}
