package github

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestWriteOutputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output")
	outputs := []Output{
		{Name: "message_id", Value: "mid-1"},
		{Name: "recipient_type", Value: "chat"},
		{Name: "recipient_id", Value: "-1001"},
	}
	if err := WriteOutputs(path, outputs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)

	// Go's regexp has no backreferences, so the delimiter is captured first and
	// then checked for a matching closing line.
	header := regexp.MustCompile(`^(\w+)<<(ghadelimiter_[0-9a-f]{32})$`)
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")

	var seen []Output
	for i := 0; i < len(lines); i++ {
		match := header.FindStringSubmatch(lines[i])
		if match == nil {
			t.Fatalf("line %d is not a heredoc header: %q\nfull output:\n%s", i, lines[i], got)
		}
		if i+2 >= len(lines) || lines[i+2] != match[2] {
			t.Fatalf("output %s is not closed by its delimiter:\n%s", match[1], got)
		}
		seen = append(seen, Output{Name: match[1], Value: lines[i+1]})
		i += 2
	}

	if len(seen) != len(outputs) {
		t.Fatalf("wrote %d outputs, want %d:\n%s", len(seen), len(outputs), got)
	}
	for i, want := range outputs {
		if seen[i] != want {
			t.Errorf("output %d = %+v, want %+v", i, seen[i], want)
		}
	}
}

func TestWriteOutputsAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output")
	if err := os.WriteFile(path, []byte("existing=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteOutputs(path, []Output{{Name: "message_id", Value: "mid"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), "existing=1\n") {
		t.Errorf("existing content was overwritten:\n%s", data)
	}
	if !strings.Contains(string(data), "message_id<<") {
		t.Errorf("new output missing:\n%s", data)
	}
}

func TestWriteOutputsMultilineValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output")
	if err := WriteOutputs(path, []Output{{Name: "message_id", Value: "a\nb"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "a\nb\n") {
		t.Errorf("multiline value not preserved:\n%s", data)
	}
}

func TestWriteOutputsWithoutPathIsNoop(t *testing.T) {
	if err := WriteOutputs("", []Output{{Name: "message_id", Value: "mid"}}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWriteOutputsUnwritablePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-dir", "output")
	if err := WriteOutputs(path, []Output{{Name: "message_id", Value: "mid"}}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestLoggerCommands(t *testing.T) {
	tests := []struct {
		name string
		call func(*Logger)
		want string
	}{
		{"mask", func(l *Logger) { l.Mask("secret") }, "::add-mask::secret\n"},
		{"mask empty is skipped", func(l *Logger) { l.Mask("") }, ""},
		{"error", func(l *Logger) { l.Errorf("boom %d", 1) }, "::error::boom 1\n"},
		{"warning", func(l *Logger) { l.Warningf("careful") }, "::warning::careful\n"},
		{"notice", func(l *Logger) { l.Noticef("fyi") }, "::notice::fyi\n"},
		{"plain", func(l *Logger) { l.Printf("sent to %s", "chat") }, "sent to chat\n"},
		{"debug off", func(l *Logger) { l.Debugf("hidden") }, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.call(&Logger{Out: &buf})
			if buf.String() != tt.want {
				t.Errorf("got %q, want %q", buf.String(), tt.want)
			}
		})
	}
}

func TestLoggerDebugEnabled(t *testing.T) {
	var buf bytes.Buffer
	(&Logger{Out: &buf, Debug: true}).Debugf("attempt %d", 2)
	if buf.String() != "debug: attempt 2\n" {
		t.Errorf("got %q", buf.String())
	}
}

// A multi-line API error must not be able to terminate the command or inject
// another one.
func TestLoggerEscapesControlCharacters(t *testing.T) {
	var buf bytes.Buffer
	(&Logger{Out: &buf}).Errorf("line one\nline two\r100%% done")

	got := buf.String()
	if strings.Count(got, "\n") != 1 || !strings.HasSuffix(got, "\n") {
		t.Errorf("newlines were not escaped: %q", got)
	}
	if got != "::error::line one%0Aline two%0D100%25 done\n" {
		t.Errorf("got %q", got)
	}
}
