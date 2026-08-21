//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/endgame-build/jira-cli/internal/api"
)

// Environment variables that configure the suite.
const (
	EnvEnable   = "JIRA_E2E"         // must be "1" for anything to run
	EnvProject  = "JIRA_E2E_PROJECT" // sandbox project key
	EnvInstance = "JIRA_INSTANCE"
	EnvUser     = "JIRA_USER"
	EnvToken    = "JIRA_TOKEN"
	EnvBin      = "JIRA_E2E_BIN"   // optional: reuse a prebuilt binary
	EnvOther    = "JIRA_E2E_OTHER" // optional: a second account ID, for conflict cases
)

// MarkerLabel is applied to every issue this suite creates. The sweeper deletes
// on this label alone, so it must never be applied to anything we do not own.
const MarkerLabel = "jira-cli-e2e"

const (
	commandTimeout = 90 * time.Second
	minCallGap     = 250 * time.Millisecond
)

// Result is one CLI invocation. Stdout and stderr are captured separately
// because several commands print warnings to stderr and still exit 0.
type Result struct {
	Args     []string
	Stdout   string
	Stderr   string
	ExitCode int
	Elapsed  time.Duration
}

// String renders the whole invocation. Every failure message embeds a Result so
// a failing test is debuggable from the log without re-running it.
func (r Result) String() string {
	return fmt.Sprintf("$ jira %s\nexit: %d\n--- stdout ---\n%s\n--- stderr ---\n%s",
		strings.Join(r.Args, " "), r.ExitCode, r.Stdout, r.Stderr)
}

// Harness is the per-test entry point.
type Harness struct {
	t        *testing.T
	bin      string
	env      []string
	API      *api.Client // fixture-side client; never used to make assertions
	Fixtures *Fixtures
	Project  string
	RunID    string
	RunLabel string
	Sandbox  *Sandbox
}

// New gates on the env vars, validates the sandbox, and returns a harness bound
// to t. A misconfigured sandbox fails; only a deliberately-disabled suite skips.
func New(t *testing.T) *Harness {
	t.Helper()
	requireEnabled(t)
	sb := preflight(t)

	h := &Harness{
		t:        t,
		bin:      binary(t),
		env:      childEnv(t),
		API:      sb.Client,
		Project:  sb.ProjectKey,
		RunID:    runID,
		RunLabel: runLabel,
		Sandbox:  sb,
	}
	h.Fixtures = newFixtures(t, h)
	return h
}

// requireEnabled skips when the suite was not explicitly asked for. This is the
// only acceptable skip for a structural reason: everything else is a failure,
// because a skipped suite and a passing suite look identical in CI output.
func requireEnabled(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("e2e: skipped under -short")
	}
	if os.Getenv(EnvEnable) != "1" {
		t.Skipf("e2e: set %s=1 to run against a real Jira instance", EnvEnable)
	}
}

// childEnv builds a hermetic environment for the CLI subprocess. Credentials
// come from env vars only, and XDG_CONFIG_HOME points at an empty directory so
// the developer's own profile, default.project, and output.format cannot reach
// the command under test.
func childEnv(t *testing.T) []string {
	t.Helper()
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + t.TempDir(),
		"XDG_CONFIG_HOME=" + t.TempDir(),
		EnvInstance + "=" + os.Getenv(EnvInstance),
		EnvUser + "=" + os.Getenv(EnvUser),
		EnvToken + "=" + os.Getenv(EnvToken),
		"NO_COLOR=1",
		"TZ=UTC", // remaining_days is computed in UTC
	}
}

// Env returns a copy of the child environment, for cases that need to mutate one
// variable (bad token, unreachable host).
func (h *Harness) Env() []string {
	out := make([]string, len(h.env))
	copy(out, h.env)
	return out
}

// Run executes the binary. A non-zero exit is data, not a failure: the caller
// decides what it means.
func (h *Harness) Run(args ...string) Result { return h.RunEnv(h.env, args...) }

// RunEnv runs with a replacement environment, for the exit-code cases that
// deliberately supply bad or missing credentials.
func (h *Harness) RunEnv(env []string, args ...string) Result {
	t := h.t
	t.Helper()
	throttle()

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, h.bin, args...)
	cmd.Env = env
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = bytes.NewReader(nil) // never let "--body-file -" block forever

	start := time.Now()
	err := cmd.Run()

	res := Result{
		Args:    args,
		Stdout:  stdout.String(),
		Stderr:  stderr.String(),
		Elapsed: time.Since(start),
	}

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		res.ExitCode = 0
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	default:
		t.Fatalf("could not execute jira %s: %v", strings.Join(args, " "), err)
	}
	if ctx.Err() != nil {
		t.Fatalf("jira %s timed out after %s\n%s", strings.Join(args, " "), commandTimeout, res)
	}
	return res
}

// MustRun fails the test unless the command exits 0.
func (h *Harness) MustRun(args ...string) Result {
	h.t.Helper()
	res := h.Run(args...)
	if res.ExitCode != 0 {
		h.t.Fatalf("expected exit 0\n%s", res)
	}
	return res
}

// RunExpectExit asserts an exact exit code and returns the result so the caller
// can also assert on the error document written to stderr.
func (h *Harness) RunExpectExit(want int, args ...string) Result {
	h.t.Helper()
	res := h.Run(args...)
	if res.ExitCode != want {
		h.t.Fatalf("expected exit %d, got %d\n%s", want, res.ExitCode, res)
	}
	return res
}

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

// CaseLabel returns a label unique to this test and this run. Assertions that
// can push a filter into the command should always use it, so the expected set
// is exactly the fixtures the test built and an exact comparison is safe even
// though the sandbox project is shared.
func (h *Harness) CaseLabel() string {
	slug := strings.ToLower(nonAlnum.ReplaceAllString(h.t.Name(), ""))
	return fmt.Sprintf("e2ecase-%s-%s", slug, h.RunID)
}

// throttle spaces CLI invocations apart. The suite authenticates as a single
// Jira user and Cloud rate limits are cost-based per user, so the cheapest way
// to stay under them is not to burst.
var (
	callMu   sync.Mutex
	lastCall time.Time
)

func throttle() {
	callMu.Lock()
	defer callMu.Unlock()
	if gap := time.Since(lastCall); gap < minCallGap {
		time.Sleep(minCallGap - gap)
	}
	lastCall = time.Now()
}

// binary compiles cmd/jira once per test process. JIRA_E2E_BIN short-circuits
// the build so CI can compile once in its own step and reuse the artifact.
var (
	binOnce sync.Once
	binPath string
	binErr  error
	binDir  string // removed by TestMain
)

func binary(t *testing.T) string {
	t.Helper()
	binOnce.Do(func() {
		if p := os.Getenv(EnvBin); p != "" {
			binPath = p
			return
		}
		binDir, binErr = os.MkdirTemp("", "jira-e2e-bin-")
		if binErr != nil {
			return
		}
		binPath = filepath.Join(binDir, "jira")
		cmd := exec.Command("go", "build", "-ldflags", "-X main.version=e2e", "-o", binPath, "./cmd/jira")
		cmd.Dir = projectRoot(t)
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			binErr = fmt.Errorf("build failed: %v\n%s", err, out)
		}
	})
	if binErr != nil {
		t.Fatalf("%v", binErr)
	}
	return binPath
}

// projectRoot walks up from the working directory to the module root, the same
// way cmd/jira/main_test.go does.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate go.mod above the working directory")
		}
		dir = parent
	}
}
