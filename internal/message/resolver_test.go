package message

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Fgeeha/max-action/internal/config"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func fullContext() Context {
	return Context{
		Actor:      "octocat",
		Workflow:   "CI",
		Repository: "Fgeeha/max-action",
		EventName:  "push",
		Ref:        "refs/heads/Master",
		SHA:        "0123456789abcdef",
		RunID:      "42",
		ServerURL:  "https://github.com",
	}
}

func TestMessageFileTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "note.txt", "from file")

	cfg := &config.Config{Message: "from input", MessageFile: "note.txt"}
	got, err := Resolve(cfg, Context{Workspace: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "from file" {
		t.Errorf("got %q, want %q", got, "from file")
	}
}

func TestMessageFilePreservesMultilineAndTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	content := "line one\n\nline three\n"
	writeFile(t, dir, "note.txt", content)

	got, err := Resolve(&config.Config{MessageFile: "note.txt"}, Context{Workspace: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != content {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestMessageFileUTF8(t *testing.T) {
	dir := t.TempDir()
	content := "Сборка завершена ✅"
	writeFile(t, dir, "note.txt", content)

	got, err := Resolve(&config.Config{MessageFile: "note.txt"}, Context{Workspace: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != content {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestMessageFileAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "note.txt", "absolute")

	got, err := Resolve(&config.Config{MessageFile: path}, Context{Workspace: "/nonexistent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "absolute" {
		t.Errorf("got %q", got)
	}
}

func TestMessageFileErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty.txt", "")
	writeFile(t, dir, "blank.txt", "   \n\t\n")
	if err := os.Mkdir(filepath.Join(dir, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "binary.txt"), []byte{0xff, 0xfe, 0xfd}, 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct{ name, file, wantErr string }{
		{"missing file", "missing.txt", "message_file not found"},
		{"directory", "adir", "message_file is a directory"},
		{"empty file", "empty.txt", "message is empty"},
		{"whitespace only", "blank.txt", "message is empty"},
		{"invalid utf8", "binary.txt", "not valid UTF-8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(&config.Config{MessageFile: tt.file}, Context{Workspace: dir})
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestMessageLengthBoundaries(t *testing.T) {
	// Multi-byte runes verify the limit counts characters, not bytes.
	tests := []struct {
		name    string
		runes   int
		wantErr bool
	}{
		{"4000 characters", 4000, false},
		{"4001 characters", 4001, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(&config.Config{Message: strings.Repeat("я", tt.runes)}, Context{})
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "exceeds MAX limit of 4000 characters") {
					t.Fatalf("error = %v, want limit error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestMessageInputMultilinePreserved(t *testing.T) {
	text := "Build completed successfully.\nCommit: abc123\n"
	got, err := Resolve(&config.Config{Message: text}, Context{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != text {
		t.Errorf("got %q, want %q", got, text)
	}
}

func TestDefaultMessage(t *testing.T) {
	got, err := Resolve(&config.Config{}, fullContext())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "octocat triggered CI\n\n" +
		"Repository: Fgeeha/max-action\n" +
		"Event: push\n" +
		"Ref: refs/heads/Master\n" +
		"Commit: 0123456\n" +
		"Run: https://github.com/Fgeeha/max-action/actions/runs/42"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestDefaultMessageUsedWhenInputIsBlank(t *testing.T) {
	got, err := Resolve(&config.Config{Message: "   \n"}, fullContext())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, "octocat triggered CI") {
		t.Errorf("expected the default template, got %q", got)
	}
}

func TestDefaultMessageSkipsMissingVariables(t *testing.T) {
	ctx := Context{Actor: "octocat", Workflow: "CI", Repository: "Fgeeha/max-action"}
	got, err := Resolve(&config.Config{}, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "octocat triggered CI\n\nRepository: Fgeeha/max-action"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultMessageOmitsRunURLWhenIncomplete(t *testing.T) {
	ctx := fullContext()
	ctx.RunID = ""
	got, err := Resolve(&config.Config{}, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "Run:") {
		t.Errorf("run URL should be omitted, got %q", got)
	}
}

func TestDefaultMessageWithEmptyContext(t *testing.T) {
	got, err := Resolve(&config.Config{}, Context{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "GitHub Actions notification" {
		t.Errorf("got %q", got)
	}
}
