package config

import (
	"strings"
	"testing"
	"time"
)

// env builds a Getenv over a map, applying the given overrides on top of a
// minimal valid input set.
func env(overrides map[string]string) Getenv {
	values := map[string]string{
		"INPUT_TOKEN":   "secret-token",
		"INPUT_CHAT_ID": "123",
		"INPUT_MESSAGE": "hello",
	}
	for k, v := range overrides {
		if v == "" {
			delete(values, k)
			continue
		}
		values[k] = v
	}
	return func(key string) string { return values[key] }
}

func TestLoadChatIDOnly(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RecipientType != RecipientChat || cfg.RecipientID != 123 {
		t.Fatalf("got %s/%d, want chat/123", cfg.RecipientType, cfg.RecipientID)
	}
}

func TestLoadUserIDOnly(t *testing.T) {
	cfg, err := Load(env(map[string]string{"INPUT_CHAT_ID": "", "INPUT_USER_ID": "456"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RecipientType != RecipientUser || cfg.RecipientID != 456 {
		t.Fatalf("got %s/%d, want user/456", cfg.RecipientType, cfg.RecipientID)
	}
}

func TestLoadNegativeChatIDAllowed(t *testing.T) {
	cfg, err := Load(env(map[string]string{"INPUT_CHAT_ID": "-1001234567890"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RecipientID != -1001234567890 {
		t.Fatalf("got %d", cfg.RecipientID)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Notify {
		t.Error("notify should default to true")
	}
	if cfg.DisableLinkPreview || cfg.Debug {
		t.Error("disable_link_preview and debug should default to false")
	}
	if cfg.Format != "" {
		t.Errorf("format should default to empty, got %q", cfg.Format)
	}
	if cfg.Timeout != 15*time.Second {
		t.Errorf("timeout = %v, want 15s", cfg.Timeout)
	}
	if cfg.Retries != 2 {
		t.Errorf("retries = %d, want 2", cfg.Retries)
	}
	if cfg.APIBaseURL.String() != DefaultAPIBaseURL {
		t.Errorf("api_base_url = %s, want %s", cfg.APIBaseURL, DefaultAPIBaseURL)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]string
		wantErr   string
	}{
		{"both targets", map[string]string{"INPUT_USER_ID": "456"}, "exactly one of chat_id or user_id"},
		{"no target", map[string]string{"INPUT_CHAT_ID": ""}, "exactly one of chat_id or user_id"},
		{"empty token", map[string]string{"INPUT_TOKEN": ""}, "token must not be empty"},
		{"blank token", map[string]string{"INPUT_TOKEN": "   "}, "token must not be empty"},
		{"non numeric chat_id", map[string]string{"INPUT_CHAT_ID": "abc"}, "chat_id must be an integer"},
		{"overflowing chat_id", map[string]string{"INPUT_CHAT_ID": "99999999999999999999"}, "chat_id must be an integer"},
		{"non numeric user_id", map[string]string{"INPUT_CHAT_ID": "", "INPUT_USER_ID": "1.5"}, "user_id must be an integer"},
		{"invalid notify", map[string]string{"INPUT_NOTIFY": "yes"}, "notify must be true or false"},
		{"invalid preview", map[string]string{"INPUT_DISABLE_LINK_PREVIEW": "1"}, "disable_link_preview must be true or false"},
		{"invalid debug", map[string]string{"INPUT_DEBUG": "on"}, "debug must be true or false"},
		{"invalid format", map[string]string{"INPUT_FORMAT": "mdown"}, "format must be one of"},
		{"timeout too low", map[string]string{"INPUT_TIMEOUT": "0"}, "timeout must be between 1 and 120"},
		{"timeout too high", map[string]string{"INPUT_TIMEOUT": "121"}, "timeout must be between 1 and 120"},
		{"timeout not a number", map[string]string{"INPUT_TIMEOUT": "fast"}, "timeout must be an integer"},
		{"retries too low", map[string]string{"INPUT_RETRIES": "-1"}, "retries must be between 0 and 5"},
		{"retries too high", map[string]string{"INPUT_RETRIES": "6"}, "retries must be between 0 and 5"},
		{"base url without scheme", map[string]string{"INPUT_API_BASE_URL": "platform-api2.max.ru"}, "absolute http or https URL"},
		{"base url with bad scheme", map[string]string{"INPUT_API_BASE_URL": "ftp://example.com"}, "absolute http or https URL"},
		{"plain http for MAX host", map[string]string{"INPUT_API_BASE_URL": "http://platform-api2.max.ru"}, "must use https for MAX API hosts"},
		{"plain http for max.ru", map[string]string{"INPUT_API_BASE_URL": "http://max.ru"}, "must use https for MAX API hosts"},
		{"plain http for MAX host with trailing dot", map[string]string{"INPUT_API_BASE_URL": "http://platform-api2.max.ru."}, "must use https for MAX API hosts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(env(tt.overrides))
			if err == nil {
				t.Fatalf("expected error, got config %+v", cfg)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), "secret-token") {
				t.Error("error leaked the token")
			}
		})
	}
}

func TestLoadAllowsHTTPForLocalMockServer(t *testing.T) {
	cfg, err := Load(env(map[string]string{"INPUT_API_BASE_URL": "http://127.0.0.1:8080/"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIBaseURL.String() != "http://127.0.0.1:8080" {
		t.Errorf("got %s, want trailing slash trimmed", cfg.APIBaseURL)
	}
}

func TestLoadKeepsMultilineMessageVerbatim(t *testing.T) {
	cfg, err := Load(env(map[string]string{"INPUT_MESSAGE": "  line one\nline two\n"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Message != "  line one\nline two\n" {
		t.Errorf("message was modified: %q", cfg.Message)
	}
}
