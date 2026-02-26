package main

import (
	"errors"
	"os"

	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"

	"github.com/endgameio/jira-cli/internal/cmd/root"
)

// version vars injected via ldflags at build time:
//
//	-X main.version=... -X main.commit=... -X main.date=...
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(run())
}

func run() int {
	f := factory.New()

	// Inject version into the root command package.
	root.Version = version

	cmd := root.NewCmdRoot(f)

	if err := cmd.Execute(); err != nil {
		// Determine whether this is a structured CLIError.
		var cliErr *clierrors.CLIError
		if !errors.As(err, &cliErr) {
			// Wrap unknown errors as GENERAL_ERROR (exit 1).
			cliErr = clierrors.NewGeneralError(err.Error())
		}

		output.OutputError(f.IOStreams.Err, cliErr, f.OutputJSON)
		return cliErr.ExitCode
	}

	return 0
}
