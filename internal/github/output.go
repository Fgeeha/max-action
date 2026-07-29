// Package github writes the workflow commands and step outputs understood by
// the GitHub Actions runner.
package github

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// Output is a single name/value pair written to GITHUB_OUTPUT.
type Output struct {
	Name  string
	Value string
}

// WriteOutputs appends outputs to the file named by GITHUB_OUTPUT. An empty
// path is a no-op, so the action still works outside a runner.
func WriteOutputs(path string, outputs []Output) error {
	if path == "" {
		return nil
	}

	var b strings.Builder
	for _, out := range outputs {
		delimiter, err := randomDelimiter()
		if err != nil {
			return err
		}
		// A heredoc keeps multi-line and special-character values intact.
		if strings.Contains(out.Value, delimiter) {
			return fmt.Errorf("cannot write output %s: value collides with its delimiter", out.Name)
		}
		fmt.Fprintf(&b, "%s<<%s\n%s\n%s\n", out.Name, delimiter, out.Value, delimiter)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("cannot open GITHUB_OUTPUT: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(b.String()); err != nil {
		return fmt.Errorf("cannot write GITHUB_OUTPUT: %w", err)
	}
	return nil
}

func randomDelimiter() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("cannot generate an output delimiter: %w", err)
	}
	return "ghadelimiter_" + hex.EncodeToString(buf), nil
}
