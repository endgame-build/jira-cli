package factory

import (
	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/config"
	"github.com/endgameio/jira-cli/internal/iostreams"
)

// NewTestFactory creates a pre-wired Factory for tests.
// All lazy accessors are pre-resolved — no credential resolution occurs.
// Pass nil for any dependency that the test doesn't need.
func NewTestFactory(ios *iostreams.IOStreams, cfg config.Config, client *api.Client) *Factory {
	f := &Factory{
		IOStreams: ios,
	}

	// Pre-fill the lazy caches so Config()/APIClient() return immediately.
	f.configOnce.Do(func() {
		f.configVal = cfg
	})

	if client != nil {
		f.clientOnce.Do(func() {
			f.clientVal = client
		})
	}

	return f
}
