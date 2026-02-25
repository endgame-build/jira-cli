package auth

import (
	"os"
	"strings"

	cliErrors "github.com/endgameio/jira-cli/internal/errors"
)

// Credentials holds resolved authentication data for the Jira API.
type Credentials struct {
	Instance string // e.g. "mysite.atlassian.net" (no scheme, no trailing slash)
	User     string // email for Basic auth
	Token    string // API token
}

// ProfileConfig is the subset of config needed for auth resolution.
// This avoids importing the full config package.
type ProfileConfig interface {
	ActiveProfile() string
	GetProfile(name string) *ProfileData
}

// ProfileData mirrors config.Profile without creating a dependency cycle.
type ProfileData struct {
	Name     string
	Instance string
	User     string
}

// Resolve resolves credentials using the chain: flags > env > profile.
// flagInstance, flagUser, flagToken are the values from --instance, --user, --token flags.
// profileName is from --profile flag (empty means "use active profile").
// cfg is used for profile lookup; tokenStore retrieves stored tokens.
func Resolve(flagInstance, flagUser, flagToken, profileName string, cfg ProfileConfig, tokenStore TokenStore) (*Credentials, error) {
	// 1. Try flags — all three must be provided.
	if flagInstance != "" || flagUser != "" || flagToken != "" {
		if flagInstance == "" || flagUser == "" || flagToken == "" {
			return nil, cliErrors.NewValidationError("All three flags required: --instance, --user, --token").
				WithSuggestion("Provide all three flags together, or use none to fall back to env vars or stored profile")
		}
		return &Credentials{
			Instance: NormalizeInstanceURL(flagInstance),
			User:     flagUser,
			Token:    flagToken,
		}, nil
	}

	// 2. Try env vars — all three must be set.
	envInstance := os.Getenv("JIRA_INSTANCE")
	envUser := os.Getenv("JIRA_USER")
	envToken := os.Getenv("JIRA_TOKEN")
	if envInstance != "" || envUser != "" || envToken != "" {
		if envInstance == "" || envUser == "" || envToken == "" {
			return nil, cliErrors.NewValidationError("All three env vars required: JIRA_INSTANCE, JIRA_USER, JIRA_TOKEN").
				WithSuggestion("Set all three environment variables together, or unset all to fall back to stored profile")
		}
		return &Credentials{
			Instance: NormalizeInstanceURL(envInstance),
			User:     envUser,
			Token:    envToken,
		}, nil
	}

	// 3. Try stored profile.
	if cfg == nil {
		return nil, cliErrors.NewAuthError("No credentials found").
			WithSuggestion("Run 'jira auth login' to store credentials")
	}

	name := profileName
	if name == "" {
		name = cfg.ActiveProfile()
	}

	profile := cfg.GetProfile(name)
	if profile == nil {
		return nil, cliErrors.NewAuthError("No credentials found").
			WithSuggestion("Run 'jira auth login' to store credentials")
	}

	token, err := tokenStore.RetrieveToken(name)
	if err != nil {
		return nil, cliErrors.NewAuthError("No credentials found").
			WithSuggestion("Run 'jira auth login' to store credentials")
	}

	return &Credentials{
		Instance: NormalizeInstanceURL(profile.Instance),
		User:     profile.User,
		Token:    token,
	}, nil
}

// NormalizeInstanceURL strips scheme, trailing slashes, and /rest/... suffix
// from an instance URL, yielding a clean hostname (e.g. "mysite.atlassian.net").
func NormalizeInstanceURL(raw string) string {
	s := raw

	// Strip scheme.
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")

	// Strip trailing slashes.
	s = strings.TrimRight(s, "/")

	// Strip /rest... suffix (with or without trailing content).
	if idx := strings.Index(s, "/rest/"); idx != -1 {
		s = s[:idx]
	} else if strings.HasSuffix(s, "/rest") {
		s = s[:len(s)-len("/rest")]
	}

	return s
}
