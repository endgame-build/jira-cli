//go:build tools

package tools

// Blank imports to keep all planned dependencies in go.mod.
// These will be moved to real imports as each package is implemented.
import (
	_ "github.com/adrg/xdg"
	_ "github.com/cli/browser"
	_ "github.com/fatih/color"
	_ "github.com/hashicorp/go-retryablehttp"
	_ "github.com/itchyny/gojq"
	_ "github.com/jedib0t/go-pretty/v6/table"
	_ "github.com/mattn/go-isatty"
	_ "github.com/pelletier/go-toml/v2"
	_ "github.com/spf13/cobra"
	_ "github.com/yuin/goldmark"
	_ "github.com/zalando/go-keyring"
)
