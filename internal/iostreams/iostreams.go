// Package iostreams provides centralized I/O with TTY detection, color control,
// and pager support. All commands use IOStreams instead of direct os.Stdout/Stderr.
package iostreams

import (
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
)

// IOStreams provides centralized access to input/output with TTY awareness,
// color control, and pager support.
type IOStreams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer

	// originalOut is the original Out before pager substitution.
	originalOut io.Writer

	// isTTY tracks whether stdout/stderr are terminal devices.
	stdoutIsTTY bool
	stderrIsTTY bool

	// colorEnabled controls whether ANSI color codes are emitted.
	colorEnabled bool

	// NoPager disables pager even when output is TTY.
	// Set per-command by commands that register --no-pager.
	NoPager bool

	// IsJSON disables pager when --json output is active.
	IsJSON bool

	// pagerCmd holds the running pager process, if any.
	pagerCmd *exec.Cmd
	// pagerPipe is the write end piped to the pager's stdin.
	pagerPipe io.WriteCloser
}

// New creates IOStreams connected to real stdin/stdout/stderr with TTY detection.
// Color is enabled when stdout is a TTY, --no-color is not set, and NO_COLOR env is absent.
func New() *IOStreams {
	stdoutTTY := isTerminal(os.Stdout)
	stderrTTY := isTerminal(os.Stderr)

	noColor := os.Getenv("NO_COLOR") != ""

	colorOn := stdoutTTY && !noColor
	if !colorOn {
		color.NoColor = true
	}

	return &IOStreams{
		In:           os.Stdin,
		Out:          os.Stdout,
		Err:          os.Stderr,
		originalOut:  os.Stdout,
		stdoutIsTTY:  stdoutTTY,
		stderrIsTTY:  stderrTTY,
		colorEnabled: colorOn,
	}
}

// isTerminal checks whether f is a terminal device.
func isTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// IsStdoutTTY reports whether stdout is a terminal.
func (s *IOStreams) IsStdoutTTY() bool {
	return s.stdoutIsTTY
}

// IsStderrTTY reports whether stderr is a terminal.
func (s *IOStreams) IsStderrTTY() bool {
	return s.stderrIsTTY
}

// ColorEnabled reports whether color output should be used.
func (s *IOStreams) ColorEnabled() bool {
	return s.colorEnabled
}

// SetColorEnabled forces color on or off (used by --no-color flag).
func (s *IOStreams) SetColorEnabled(on bool) {
	s.colorEnabled = on
	color.NoColor = !on
}

// --- Color scheme methods ---
// All return plain text when color is disabled.

// Green formats text in green.
func (s *IOStreams) Green(t string) string {
	if !s.colorEnabled {
		return t
	}
	return color.GreenString("%s", t)
}

// Yellow formats text in yellow.
func (s *IOStreams) Yellow(t string) string {
	if !s.colorEnabled {
		return t
	}
	return color.YellowString("%s", t)
}

// Red formats text in red.
func (s *IOStreams) Red(t string) string {
	if !s.colorEnabled {
		return t
	}
	return color.RedString("%s", t)
}

// Bold formats text in bold.
func (s *IOStreams) Bold(t string) string {
	if !s.colorEnabled {
		return t
	}
	return color.New(color.Bold).Sprint(t)
}

// Cyan formats text in cyan.
func (s *IOStreams) Cyan(t string) string {
	if !s.colorEnabled {
		return t
	}
	return color.CyanString("%s", t)
}

// --- Pager support ---

// pagerName resolves the pager program: JIRA_PAGER > PAGER > "less".
func pagerName() string {
	if p := os.Getenv("JIRA_PAGER"); p != "" {
		return p
	}
	if p := os.Getenv("PAGER"); p != "" {
		return p
	}
	return "less"
}

// StartPager spawns the pager and redirects Out through it.
// No pager when: not TTY, NoPager set, or IsJSON set.
func (s *IOStreams) StartPager() {
	if !s.stdoutIsTTY || s.NoPager || s.IsJSON {
		return
	}

	pager := pagerName()
	parts := strings.Fields(pager)
	if len(parts) == 0 {
		return
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdout = s.originalOut
	cmd.Stderr = s.Err

	pipe, err := cmd.StdinPipe()
	if err != nil {
		return
	}

	if err := cmd.Start(); err != nil {
		return
	}

	s.pagerCmd = cmd
	s.pagerPipe = pipe
	s.Out = pipe
}

// StopPager closes the pager pipe and waits for the process to exit.
func (s *IOStreams) StopPager() {
	if s.pagerPipe != nil {
		_ = s.pagerPipe.Close()
	}
	if s.pagerCmd != nil {
		_ = s.pagerCmd.Wait()
	}
	s.pagerPipe = nil
	s.pagerCmd = nil
	s.Out = s.originalOut
}
