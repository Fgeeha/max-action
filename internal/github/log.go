package github

import (
	"fmt"
	"io"
	"strings"
)

// Logger emits GitHub Actions workflow commands.
type Logger struct {
	Out   io.Writer
	Debug bool
}

// Mask registers a secret so the runner redacts it from every later log line.
// Call this before anything else can print the value.
func (l *Logger) Mask(secret string) {
	if secret == "" {
		return
	}
	l.command("add-mask", secret)
}

// Errorf emits an error annotation.
func (l *Logger) Errorf(format string, args ...any) {
	l.command("error", fmt.Sprintf(format, args...))
}

// Warningf emits a warning annotation.
func (l *Logger) Warningf(format string, args ...any) {
	l.command("warning", fmt.Sprintf(format, args...))
}

// Noticef emits a notice annotation.
func (l *Logger) Noticef(format string, args ...any) {
	l.command("notice", fmt.Sprintf(format, args...))
}

// Debugf prints a diagnostic line when debug output is enabled.
func (l *Logger) Debugf(format string, args ...any) {
	if !l.Debug {
		return
	}
	fmt.Fprintf(l.Out, "debug: %s\n", escapeData(fmt.Sprintf(format, args...)))
}

// Printf prints a plain log line.
func (l *Logger) Printf(format string, args ...any) {
	fmt.Fprintf(l.Out, format+"\n", args...)
}

func (l *Logger) command(name, value string) {
	fmt.Fprintf(l.Out, "::%s::%s\n", name, escapeData(value))
}

// escapeData applies the runner's encoding so control characters cannot break
// out of a workflow command or forge a second one.
func escapeData(s string) string {
	return strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
	).Replace(s)
}
