package iostreams

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestTest_ReturnsNonTTY(t *testing.T) {
	tio := Test()

	if tio.IsStdoutTTY() {
		t.Error("Test() stdout should not be TTY")
	}
	if tio.IsStderrTTY() {
		t.Error("Test() stderr should not be TTY")
	}
	if tio.ColorEnabled() {
		t.Error("Test() should have color disabled")
	}
}

func TestTest_CapturesOutput(t *testing.T) {
	tio := Test()

	fmt.Fprint(tio.Out, "hello stdout")
	fmt.Fprint(tio.Err, "hello stderr")

	if got := tio.OutBuf.String(); got != "hello stdout" {
		t.Errorf("OutBuf = %q, want %q", got, "hello stdout")
	}
	if got := tio.ErrBuf.String(); got != "hello stderr" {
		t.Errorf("ErrBuf = %q, want %q", got, "hello stderr")
	}
}

func TestTest_ReadsInput(t *testing.T) {
	tio := Test()
	tio.InBuf.WriteString("user input")

	buf := make([]byte, 10)
	n, err := tio.In.Read(buf)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if got := string(buf[:n]); got != "user input" {
		t.Errorf("In = %q, want %q", got, "user input")
	}
}

func TestColorEnabled_DefaultOff_InTest(t *testing.T) {
	tio := Test()
	if tio.ColorEnabled() {
		t.Error("Test IOStreams should have color disabled by default")
	}
}

func TestSetColorEnabled(t *testing.T) {
	tio := Test()

	tio.SetColorEnabled(true)
	if !tio.ColorEnabled() {
		t.Error("expected color enabled after SetColorEnabled(true)")
	}

	tio.SetColorEnabled(false)
	if tio.ColorEnabled() {
		t.Error("expected color disabled after SetColorEnabled(false)")
	}
}

func TestColorMethods_NoColor(t *testing.T) {
	tio := Test()

	tests := []struct {
		name   string
		method func(string) string
		input  string
	}{
		{"Green", tio.Green, "ok"},
		{"Yellow", tio.Yellow, "warn"},
		{"Red", tio.Red, "err"},
		{"Bold", tio.Bold, "title"},
		{"Cyan", tio.Cyan, "info"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.method(tt.input)
			if got != tt.input {
				t.Errorf("%s(%q) = %q, want plain %q (color disabled)", tt.name, tt.input, got, tt.input)
			}
		})
	}
}

func TestColorMethods_WithColor(t *testing.T) {
	tio := Test()
	tio.SetColorEnabled(true)

	tests := []struct {
		name   string
		method func(string) string
		input  string
	}{
		{"Green", tio.Green, "ok"},
		{"Yellow", tio.Yellow, "warn"},
		{"Red", tio.Red, "err"},
		{"Bold", tio.Bold, "title"},
		{"Cyan", tio.Cyan, "info"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.method(tt.input)
			// When color enabled, output should contain ANSI escape and the original text
			if !strings.Contains(got, tt.input) {
				t.Errorf("%s(%q) = %q, should contain %q", tt.name, tt.input, got, tt.input)
			}
			if got == tt.input {
				t.Errorf("%s(%q) = %q, expected ANSI codes when color enabled", tt.name, tt.input, got)
			}
		})
	}
}

func TestNoPager_PreventsStartPager(t *testing.T) {
	tio := Test()
	tio.NoPager = true

	original := tio.Out
	tio.StartPager() // should be no-op: not TTY + NoPager
	if tio.Out != original {
		t.Error("StartPager should not redirect Out when NoPager is set")
	}
}

func TestIsJSON_PreventsStartPager(t *testing.T) {
	tio := Test()
	tio.IsJSON = true

	original := tio.Out
	tio.StartPager()
	if tio.Out != original {
		t.Error("StartPager should not redirect Out when IsJSON is set")
	}
}

func TestStartPager_NonTTY_Noop(t *testing.T) {
	tio := Test()

	original := tio.Out
	tio.StartPager()
	if tio.Out != original {
		t.Error("StartPager should be no-op when not TTY")
	}
}

func TestStopPager_NilSafe(t *testing.T) {
	tio := Test()
	// StopPager with no active pager should not panic
	tio.StopPager()
}

func TestNew_NoColor_Env(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	ios := New()
	if ios.ColorEnabled() {
		t.Error("New() should have color disabled when NO_COLOR env is set")
	}
}

func TestPagerName_Defaults(t *testing.T) {
	tests := []struct {
		name     string
		jiraPgr  string
		pagerEnv string
		want     string
	}{
		{"JIRA_PAGER wins", "bat", "more", "bat"},
		{"PAGER fallback", "", "more", "more"},
		{"default less", "", "", "less"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear both env vars, then set as needed
			t.Setenv("JIRA_PAGER", tt.jiraPgr)
			t.Setenv("PAGER", tt.pagerEnv)

			got := pagerName()
			if got != tt.want {
				t.Errorf("pagerName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNew_PipeSafety(t *testing.T) {
	// When stdout is not a TTY (e.g., piped), color should be off.
	// We can't fully simulate non-TTY in a test process, but we can
	// verify that New() at least runs and produces a valid IOStreams.
	// In CI (where stdout is a pipe), this validates pipe-safety.
	ios := New()
	if ios.Out == nil || ios.Err == nil || ios.In == nil {
		t.Error("New() should set all IO fields")
	}
}

func TestStartPager_WithTTY_SpawnsCat(t *testing.T) {
	// Simulate a TTY IOStreams and use "cat" as pager to exercise the
	// full pager lifecycle without requiring an actual terminal.
	t.Setenv("JIRA_PAGER", "cat")

	outBuf := &bytes.Buffer{}
	ios := &IOStreams{
		In:           &bytes.Buffer{},
		Out:          outBuf,
		Err:          os.Stderr,
		originalOut:  outBuf,
		stdoutIsTTY:  true,
		stderrIsTTY:  false,
		colorEnabled: false,
	}

	ios.StartPager()

	if ios.pagerCmd == nil {
		t.Fatal("expected pager process to start")
	}
	if ios.pagerPipe == nil {
		t.Fatal("expected pager pipe to be created")
	}

	// Write through the pager pipe
	fmt.Fprint(ios.Out, "paged content")
	ios.StopPager()

	// After stop, Out should be restored to originalOut
	if ios.Out != ios.originalOut {
		t.Error("StopPager should restore original Out")
	}

	// cat should have passed content through to outBuf
	if got := outBuf.String(); !strings.Contains(got, "paged content") {
		t.Errorf("pager output = %q, expected to contain %q", got, "paged content")
	}
}
