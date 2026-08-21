//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/endgame-build/jira-cli/internal/api"
)

// orphanGrace is how old an issue must be before the pre-run sweep will delete
// it. It exists so a concurrent run's fixtures are never destroyed mid-test.
const orphanGrace = 2 * time.Hour

var (
	runID    string // e.g. "20260821t143012-7f3a"
	runLabel string // "e2e-run-<runID>"
)

func TestMain(m *testing.M) {
	// testing.Short() reads a flag, so the flags must be parsed before it is
	// consulted. TestMain runs before the testing package does this itself.
	flag.Parse()

	if os.Getenv(EnvEnable) != "1" || testing.Short() {
		// Every test calls New(t), which skips. Nothing to set up or tear down.
		os.Exit(m.Run())
	}

	runID = newRunID()
	runLabel = "e2e-run-" + runID

	client, err := bootstrapClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: cannot build API client: %v\n\n%s\n", err, setupHint)
		os.Exit(1)
	}
	project := os.Getenv(EnvProject)
	if project == "" {
		fmt.Fprintf(os.Stderr, "e2e: %s is not set\n\n%s\n", EnvProject, setupHint)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "e2e: instance=%s project=%s run=%s\n",
		os.Getenv(EnvInstance), project, runID)

	// Runs killed by a timeout, a crash, or a cancelled CI job leave issues
	// behind. Clear anything old enough to be certainly abandoned.
	if n := sweepLabel(client, project, MarkerLabel, orphanGrace); n > 0 {
		fmt.Fprintf(os.Stderr, "e2e: swept %d orphan(s) from earlier runs\n", n)
	}
	installInterruptSweeper(client, project)

	code := m.Run()

	// Anything still carrying this run's label is a per-test cleanup bug.
	// Delete it, then fail the run so the bug is not quietly tolerated.
	if n := sweepLabel(client, project, runLabel, 0); n > 0 {
		fmt.Fprintf(os.Stderr, "e2e: LEAK — swept %d issue(s) still labelled %s\n", n, runLabel)
		code = 1
	}

	if binDir != "" {
		_ = os.RemoveAll(binDir)
	}
	os.Exit(code)
}

func newRunID() string {
	var b [2]byte
	_, _ = rand.Read(b[:])
	return time.Now().UTC().Format("20060102t150405") + "-" + hex.EncodeToString(b[:])
}

// sweepLabel deletes every issue in the project carrying label, optionally
// restricted to those older than minAge.
//
// It always requires MarkerLabel as well, so a typo in a run ID can never reach
// an issue the suite did not create. That safety property is the reason the
// sweeper is allowed to exist at all.
func sweepLabel(c *api.Client, project, label string, minAge time.Duration) int {
	ctx := context.Background()

	jql := fmt.Sprintf("project = %q AND labels = %q", project, MarkerLabel)
	if label != MarkerLabel {
		jql += fmt.Sprintf(" AND labels = %q", label)
	}
	if minAge > 0 {
		jql += fmt.Sprintf(" AND created <= -%dm", int(minAge.Minutes()))
	}

	res, err := c.SearchIssues(ctx, &api.SearchOptions{
		JQL:        jql,
		Fields:     []string{"key"},
		MaxResults: 200,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: sweep search failed: %v\n", err)
		return 0
	}

	n := 0
	for _, issue := range res.Issues {
		if err := c.DeleteIssue(ctx, issue.Key, true); err == nil {
			n++
		}
	}
	return n
}

// installInterruptSweeper handles Ctrl-C. Go's test runner does not run
// t.Cleanup on a signal, so without this every issue created by an interrupted
// run leaks until the next sweep.
func installInterruptSweeper(c *api.Client, project string) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		fmt.Fprintf(os.Stderr, "\ne2e: interrupted — sweeping %s\n", runLabel)
		sweepLabel(c, project, runLabel, 0)
		os.Exit(130)
	}()
}

// TestE2E_SWEEP deletes every issue in the sandbox carrying MarkerLabel,
// regardless of age. It exists for when a run died badly and you want a clean
// project now, and is gated on its own variable so it cannot fire by accident.
func TestE2E_SWEEP(t *testing.T) {
	requireEnabled(t)
	if os.Getenv("JIRA_E2E_SWEEP") != "1" {
		t.Skip("set JIRA_E2E_SWEEP=1 to delete every e2e issue in the sandbox")
	}
	sb := preflight(t)
	t.Logf("swept %d issue(s)", sweepLabel(sb.Client, sb.ProjectKey, MarkerLabel, 0))
}
