// Package message resolves the text to send from the action inputs or, as a
// fallback, from the GitHub Actions context.
package message

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Fgeeha/max-action/internal/config"
)

// Context carries the GitHub Actions variables used by the default template.
// Any field may be empty: the template skips lines it cannot fill.
type Context struct {
	Actor      string
	Workflow   string
	Repository string
	EventName  string
	Ref        string
	SHA        string
	RunID      string
	ServerURL  string
	Workspace  string
}

// ContextFromEnv reads the GitHub Actions context from the environment.
func ContextFromEnv(getenv func(string) string) Context {
	return Context{
		Actor:      getenv("GITHUB_ACTOR"),
		Workflow:   getenv("GITHUB_WORKFLOW"),
		Repository: getenv("GITHUB_REPOSITORY"),
		EventName:  getenv("GITHUB_EVENT_NAME"),
		Ref:        getenv("GITHUB_REF"),
		SHA:        getenv("GITHUB_SHA"),
		RunID:      getenv("GITHUB_RUN_ID"),
		ServerURL:  getenv("GITHUB_SERVER_URL"),
		Workspace:  getenv("GITHUB_WORKSPACE"),
	}
}

// Resolve returns the message text, applying the documented precedence:
// message_file, then message, then the default template.
func Resolve(cfg *config.Config, ctx Context) (string, error) {
	text, err := resolveText(cfg, ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", errors.New("message is empty")
	}
	if n := utf8.RuneCountInString(text); n > config.MaxMessageRunes {
		return "", fmt.Errorf("message exceeds MAX limit of %d characters (got %d)", config.MaxMessageRunes, n)
	}
	return text, nil
}

func resolveText(cfg *config.Config, ctx Context) (string, error) {
	if cfg.MessageFile != "" {
		return readMessageFile(cfg.MessageFile, ctx.Workspace)
	}
	if strings.TrimSpace(cfg.Message) != "" {
		return cfg.Message, nil
	}
	return defaultMessage(ctx), nil
}

// readMessageFile reads a UTF-8 file, resolving relative paths against the
// workspace. Trailing newlines are preserved as written.
func readMessageFile(path, workspace string) (string, error) {
	if !filepath.IsAbs(path) && workspace != "" {
		path = filepath.Join(workspace, path)
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("message_file not found: %s", path)
		}
		return "", fmt.Errorf("message_file cannot be read: %s", path)
	}
	if info.IsDir() {
		return "", fmt.Errorf("message_file is a directory: %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("message_file cannot be read: %s", path)
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("message_file is not valid UTF-8: %s", path)
	}
	return string(data), nil
}

// defaultMessage builds a compact summary from the GitHub Actions context,
// omitting any line whose value is unavailable.
func defaultMessage(ctx Context) string {
	var b strings.Builder

	switch {
	case ctx.Actor != "" && ctx.Workflow != "":
		fmt.Fprintf(&b, "%s triggered %s\n\n", ctx.Actor, ctx.Workflow)
	case ctx.Workflow != "":
		fmt.Fprintf(&b, "%s\n\n", ctx.Workflow)
	}

	for _, line := range []struct{ label, value string }{
		{"Repository", ctx.Repository},
		{"Event", ctx.EventName},
		{"Ref", ctx.Ref},
		{"Commit", shortSHA(ctx.SHA)},
		{"Run", runURL(ctx)},
	} {
		if line.value != "" {
			fmt.Fprintf(&b, "%s: %s\n", line.label, line.value)
		}
	}

	text := strings.TrimRight(b.String(), "\n")
	if text == "" {
		return "GitHub Actions notification"
	}
	return text
}

// shortSHA trims a commit SHA for display only.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// runURL builds the workflow run link only when every part is present.
func runURL(ctx Context) string {
	if ctx.ServerURL == "" || ctx.Repository == "" || ctx.RunID == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/actions/runs/%s", strings.TrimSuffix(ctx.ServerURL, "/"), ctx.Repository, ctx.RunID)
}
