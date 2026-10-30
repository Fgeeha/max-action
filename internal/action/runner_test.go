package action

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const token = "secret-token-value"

// harness runs the action against a stub MAX server.
type harness struct {
	inputs     map[string]string
	outputPath string
	stdout     bytes.Buffer
	requests   int
	lastBody   string
	lastQuery  string
	lastAuth   string
}

func newHarness(t *testing.T, handler http.HandlerFunc) (*harness, func()) {
	t.Helper()
	h := &harness{outputPath: filepath.Join(t.TempDir(), "output")}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		h.requests++
		h.lastBody = string(body)
		h.lastQuery = r.URL.RawQuery
		h.lastAuth = r.Header.Get("Authorization")
		handler(w, r)
	}))

	h.inputs = map[string]string{
		"INPUT_TOKEN":        token,
		"INPUT_CHAT_ID":      "123",
		"INPUT_MESSAGE":      "hello from CI",
		"INPUT_API_BASE_URL": server.URL,
		"GITHUB_OUTPUT":      h.outputPath,
	}
	return h, server.Close
}

func (h *harness) run() error {
	return Run(context.Background(), Env{
		Getenv:  func(key string) string { return h.inputs[key] },
		Stdout:  &h.stdout,
		Version: "test",
	})
}

func (h *harness) outputs(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(h.outputPath)
	if err != nil {
		t.Fatalf("read outputs: %v", err)
	}
	return string(data)
}

func sendOK(w http.ResponseWriter, _ *http.Request) {
	io.WriteString(w, `{"message":{"body":{"mid":"mid-42"}}}`)
}

func TestRunSuccess(t *testing.T) {
	h, closeServer := newHarness(t, sendOK)
	defer closeServer()

	if err := h.run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if h.lastAuth != token {
		t.Errorf("Authorization = %q", h.lastAuth)
	}
	if !strings.Contains(h.lastBody, "hello from CI") {
		t.Errorf("body = %s", h.lastBody)
	}
	if h.lastQuery != "chat_id=123" {
		t.Errorf("query = %s", h.lastQuery)
	}

	out := h.outputs(t)
	for _, want := range []string{"message_id<<", "mid-42", "recipient_type<<", "chat", "recipient_id<<", "123"} {
		if !strings.Contains(out, want) {
			t.Errorf("outputs missing %q:\n%s", want, out)
		}
	}

	stdout := h.stdout.String()
	if !strings.Contains(stdout, "::add-mask::"+token) {
		t.Errorf("token was not masked:\n%s", stdout)
	}
	if !strings.Contains(stdout, "MAX message sent successfully to chat 123") {
		t.Errorf("missing success line:\n%s", stdout)
	}
}

func TestRunUsesDefaultMessage(t *testing.T) {
	h, closeServer := newHarness(t, sendOK)
	defer closeServer()

	delete(h.inputs, "INPUT_MESSAGE")
	h.inputs["GITHUB_ACTOR"] = "octocat"
	h.inputs["GITHUB_WORKFLOW"] = "CI"
	h.inputs["GITHUB_REPOSITORY"] = "Fgeeha/max-action"

	if err := h.run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(h.lastBody, "octocat triggered CI") {
		t.Errorf("default message not used, body = %s", h.lastBody)
	}
}

func TestRunReadsMessageFile(t *testing.T) {
	h, closeServer := newHarness(t, sendOK)
	defer closeServer()

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("from the file"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.inputs["GITHUB_WORKSPACE"] = workspace
	h.inputs["INPUT_MESSAGE_FILE"] = "note.txt"

	if err := h.run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(h.lastBody, "from the file") {
		t.Errorf("body = %s", h.lastBody)
	}
}

func TestRunPassesOptionalParameters(t *testing.T) {
	h, closeServer := newHarness(t, sendOK)
	defer closeServer()

	h.inputs["INPUT_FORMAT"] = "markdown"
	h.inputs["INPUT_NOTIFY"] = "false"
	h.inputs["INPUT_DISABLE_LINK_PREVIEW"] = "true"

	if err := h.run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(h.lastBody, `"format":"markdown"`) || !strings.Contains(h.lastBody, `"notify":false`) {
		t.Errorf("body = %s", h.lastBody)
	}
	if !strings.Contains(h.lastQuery, "disable_link_preview=true") {
		t.Errorf("query = %s", h.lastQuery)
	}
}

func TestRunUserRecipient(t *testing.T) {
	h, closeServer := newHarness(t, sendOK)
	defer closeServer()

	delete(h.inputs, "INPUT_CHAT_ID")
	h.inputs["INPUT_USER_ID"] = "789"

	if err := h.run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.lastQuery != "user_id=789" {
		t.Errorf("query = %s", h.lastQuery)
	}
	if !strings.Contains(h.outputs(t), "user") {
		t.Errorf("recipient_type output missing:\n%s", h.outputs(t))
	}
}

func TestRunSucceedsWithoutMessageID(t *testing.T) {
	h, closeServer := newHarness(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	defer closeServer()

	if err := h.run(); err != nil {
		t.Fatalf("a success without mid must not fail: %v", err)
	}
	if !strings.Contains(h.outputs(t), "message_id<<") {
		t.Errorf("message_id output should still be written:\n%s", h.outputs(t))
	}
}

// A delivered message must not be reported as a failure just because the
// outputs could not be recorded: a retry would send it twice.
func TestRunWarnsWhenOutputsCannotBeWritten(t *testing.T) {
	h, closeServer := newHarness(t, sendOK)
	defer closeServer()

	h.inputs["GITHUB_OUTPUT"] = filepath.Join(t.TempDir(), "missing-dir", "output")

	if err := h.run(); err != nil {
		t.Fatalf("a delivered message must still succeed: %v", err)
	}
	stdout := h.stdout.String()
	if !strings.Contains(stdout, "::warning::message sent, but the outputs could not be recorded") {
		t.Errorf("expected a warning:\n%s", stdout)
	}
	if strings.Contains(stdout, "::error::") {
		t.Errorf("must not emit an error annotation:\n%s", stdout)
	}
}

func TestRunWithoutGithubOutput(t *testing.T) {
	h, closeServer := newHarness(t, sendOK)
	defer closeServer()

	delete(h.inputs, "GITHUB_OUTPUT")

	if err := h.run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(h.stdout.String(), "sent successfully") {
		t.Errorf("stdout = %s", h.stdout.String())
	}
}

func TestRunConfigErrorsSkipTheRequest(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(map[string]string)
		wantText string
	}{
		{"no target", func(in map[string]string) { delete(in, "INPUT_CHAT_ID") }, "exactly one of chat_id or user_id"},
		{"both targets", func(in map[string]string) { in["INPUT_USER_ID"] = "9" }, "exactly one of chat_id or user_id"},
		{"empty token", func(in map[string]string) { in["INPUT_TOKEN"] = "" }, "token must not be empty"},
		{"bad format", func(in map[string]string) { in["INPUT_FORMAT"] = "bbcode" }, "format must be one of"},
		{"message too long", func(in map[string]string) { in["INPUT_MESSAGE"] = strings.Repeat("x", 4001) }, "exceeds MAX limit"},
		{"missing message file", func(in map[string]string) { in["INPUT_MESSAGE_FILE"] = "/nope/missing.txt" }, "message_file not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, closeServer := newHarness(t, sendOK)
			defer closeServer()
			tt.mutate(h.inputs)

			if err := h.run(); err == nil {
				t.Fatal("expected an error")
			}
			if h.requests != 0 {
				t.Errorf("made %d requests, want 0", h.requests)
			}
			if !strings.Contains(h.stdout.String(), "::error::") {
				t.Errorf("missing error annotation:\n%s", h.stdout.String())
			}
			if !strings.Contains(h.stdout.String(), tt.wantText) {
				t.Errorf("stdout = %s, want it to mention %q", h.stdout.String(), tt.wantText)
			}
			if _, err := os.Stat(h.outputPath); err == nil {
				t.Error("outputs must not be written on failure")
			}
		})
	}
}

func TestRunAPIErrorFailsAndNeverLeaksToken(t *testing.T) {
	h, closeServer := newHarness(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"code":"auth","message":"invalid token `+token+`"}`)
	})
	defer closeServer()

	if err := h.run(); err == nil {
		t.Fatal("expected an error")
	}

	stdout := h.stdout.String()
	if !strings.Contains(stdout, "::error::MAX API returned HTTP 401") {
		t.Errorf("missing error annotation:\n%s", stdout)
	}
	// The add-mask line necessarily contains the token; nothing else may.
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if strings.HasPrefix(line, "::add-mask::") {
			continue
		}
		if strings.Contains(line, token) {
			t.Errorf("token leaked into: %s", line)
		}
	}
}

func TestRunDebugStaysSafe(t *testing.T) {
	h, closeServer := newHarness(t, sendOK)
	defer closeServer()

	h.inputs["INPUT_DEBUG"] = "true"
	h.inputs["INPUT_MESSAGE"] = "highly confidential release notes"

	if err := h.run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stdout := h.stdout.String()
	if !strings.Contains(stdout, "debug:") {
		t.Errorf("expected debug output:\n%s", stdout)
	}
	if strings.Contains(stdout, "highly confidential") {
		t.Errorf("debug output leaked the message body:\n%s", stdout)
	}
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "::add-mask::") {
			continue
		}
		if strings.Contains(line, token) {
			t.Errorf("debug output leaked the token: %s", line)
		}
	}
	if !strings.Contains(stdout, "non-default MAX API host") {
		t.Errorf("debug output should name the custom host:\n%s", stdout)
	}
}

func TestRunUploadsAttachmentFile(t *testing.T) {
	var uploadAuth string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/uploads":
			io.WriteString(w, `{"url":"`+server.URL+`/upload-host"}`)
		case "/upload-host":
			uploadAuth = r.Header.Get("Authorization")
			io.WriteString(w, `{"token":"file-token"}`)
		default:
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"token":"file-token"`) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			io.WriteString(w, `{"message":{"body":{"mid":"mid-9"}}}`)
		}
	}))
	defer server.Close()

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "docs.zip"), []byte("zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	h, closeStub := newHarness(t, func(http.ResponseWriter, *http.Request) {})
	defer closeStub()
	h.inputs["INPUT_API_BASE_URL"] = server.URL
	h.inputs["INPUT_ATTACHMENT_FILE"] = "docs.zip"
	h.inputs["GITHUB_WORKSPACE"] = workspace

	if err := h.run(); err != nil {
		t.Fatalf("run: %v\n%s", err, h.stdout.String())
	}
	if uploadAuth != "" {
		t.Fatal("token reached the upload host")
	}
	if !strings.Contains(h.outputs(t), "\nmid-9\n") {
		t.Fatalf("outputs = %q", h.outputs(t))
	}
}

func TestRunMissingAttachmentSkipsTheRequest(t *testing.T) {
	h, closeServer := newHarness(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{}`)
	})
	defer closeServer()
	h.inputs["INPUT_ATTACHMENT_FILE"] = "missing.zip"
	h.inputs["GITHUB_WORKSPACE"] = t.TempDir()

	err := h.run()
	if err == nil || !strings.Contains(err.Error(), "attachment_file not found") {
		t.Fatalf("err = %v", err)
	}
	if h.requests != 0 {
		t.Fatalf("requests = %d, want 0", h.requests)
	}
}
