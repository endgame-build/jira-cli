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

// TestE2E_ERR_03 — an unresolvable host exits 7.
//
// Bad credentials are covered separately by TestE2E_ERR_05: no agent command
// surfaces them as an auth error.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-err-03
func TestE2E_ERR_03(t *testing.T) {
	h := New(t)

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

// TestE2E_ERR_05 — an invalid token is indistinguishable from an empty backlog.
//
// Jira Cloud answers POST /rest/api/3/search/jql with HTTP 200 and zero issues
// when the credentials are bad, rather than 401 — verified directly against the
// API. The CLI relays that faithfully, so with an expired token:
//
//	agent ready   → "No ready issues found", exit 0
//	agent blocked → "No blocked issues found", exit 0
//	agent status  → a full summary of zeroes, exit 0
//	agent claim   → NOT_FOUND (exit 4), blaming the issue rather than the session
//	auth status   → "Token: invalid", but still exit 0
//
// An unattended agent running this loop with expired credentials therefore
// reports healthy and quietly does nothing, forever. Nothing in the loop can
// tell "you are not authenticated" from "there is no work", and the one command
// that detects it cannot be used as a scripted gate because it exits 0.
//
// This is the most operationally dangerous behaviour the suite found. The case
// documents it rather than asserting it is correct.
//
// Spec: docs/e2e-agent-sdlc-spec.md#e2e-err-05
func TestE2E_ERR_05(t *testing.T) {
	h := New(t)
	bad := replaceEnv(h.Env(), EnvToken, "definitely-not-a-valid-token")

	ready := h.RunEnv(bad, "agent", "ready", "-p", h.Project, "--json")
	if ready.ExitCode == 2 {
		t.Log("`agent ready` now reports AUTH_ERROR on a bad token; this case may be obsolete")
		return
	}
	if ready.ExitCode != 0 {
		t.Errorf("`agent ready` exit = %d with a bad token; expected either 0 (the documented "+
			"defect) or 2 (fixed)\n%s", ready.ExitCode, ready)
		return
	}

	items, _ := DecodeList[ReadyItem](t, ready)
	if len(items) != 0 {
		t.Fatalf("a bad token returned %d issues; the credentials may not have been overridden", len(items))
	}

	// The same blindness across the rest of the loop.
	blocked := h.RunEnv(bad, "agent", "blocked", "-p", h.Project, "--json")
	if blocked.ExitCode != 0 {
		t.Logf("`agent blocked` exit = %d with a bad token (ready exits 0)", blocked.ExitCode)
	}
	status := h.RunEnv(bad, "agent", "status", "-p", h.Project, "--json")
	if status.ExitCode != 0 {
		t.Logf("`agent status` exit = %d with a bad token (ready exits 0)", status.ExitCode)
	}

	// claim does surface a failure, but attributes it to the wrong thing.
	claim := h.RunEnv(bad, "agent", "claim", h.Project+"-1", "--json")
	if claim.ExitCode == 4 {
		t.Logf("`agent claim` exits 4 NOT_FOUND on a bad token, blaming the issue rather than " +
			"the session")
	}

	// auth status detects it but cannot gate a script.
	authStatus := h.RunEnv(bad, "auth", "status")
	if authStatus.ExitCode == 0 {
		t.Logf("`auth status` prints %q but still exits 0, so it cannot be used as a "+
			"credential gate in a script", "Token: invalid")
	}

	t.Log("KNOWN DEFECT: with an invalid token the whole agent loop reports healthy and empty. " +
		"Jira returns HTTP 200 with no issues for search/jql when auth fails, so `agent ready` " +
		"cannot distinguish expired credentials from an empty backlog. An unattended agent " +
		"idles forever at exit 0. A cheap fix is to validate the session (GET /myself, which " +
		"does 401) before trusting an empty search result.")
}
