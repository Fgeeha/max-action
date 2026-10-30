// Package config reads and validates GitHub Actions inputs for the action.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Recipient types reported through the recipient_type output.
const (
	RecipientChat = "chat"
	RecipientUser = "user"
)

// MaxMessageRunes is the MAX API limit for the text field.
const MaxMessageRunes = 4000

// DefaultAPIBaseURL is the current MAX Bot API endpoint.
const DefaultAPIBaseURL = "https://platform-api2.max.ru"

const (
	minTimeoutSeconds = 1
	maxTimeoutSeconds = 120
	maxRetries        = 5
)

// Config holds validated action inputs.
type Config struct {
	Token              string
	RecipientType      string
	RecipientID        int64
	Message            string
	MessageFile        string
	AttachmentFile     string
	Format             string
	Notify             bool
	DisableLinkPreview bool
	APIBaseURL         *url.URL
	Timeout            time.Duration
	Retries            int
	Debug              bool
}

// Getenv reads an environment variable, matching os.Getenv.
type Getenv func(string) string

// Load reads INPUT_* variables provided by the GitHub Actions runner and
// validates them. It never includes the token in returned errors.
func Load(getenv Getenv) (*Config, error) {
	input := func(name string) string {
		return strings.TrimSpace(getenv("INPUT_" + strings.ToUpper(name)))
	}

	cfg := &Config{
		// message keeps its original formatting, only inputs parsed as values
		// are trimmed.
		Message:        getenv("INPUT_MESSAGE"),
		MessageFile:    input("message_file"),
		AttachmentFile: input("attachment_file"),
		Format:         input("format"),
	}

	cfg.Token = input("token")
	if cfg.Token == "" {
		return nil, errors.New("token must not be empty")
	}

	var err error
	if cfg.RecipientType, cfg.RecipientID, err = parseRecipient(input("chat_id"), input("user_id")); err != nil {
		return nil, err
	}

	switch cfg.Format {
	case "", "markdown", "html":
	default:
		return nil, errors.New("format must be one of: markdown, html, or empty")
	}

	if cfg.Notify, err = parseBool(input("notify"), "notify", true); err != nil {
		return nil, err
	}
	if cfg.DisableLinkPreview, err = parseBool(input("disable_link_preview"), "disable_link_preview", false); err != nil {
		return nil, err
	}
	if cfg.Debug, err = parseBool(input("debug"), "debug", false); err != nil {
		return nil, err
	}

	if cfg.APIBaseURL, err = parseBaseURL(input("api_base_url")); err != nil {
		return nil, err
	}

	seconds, err := parseIntInRange(input("timeout"), "timeout", 15, minTimeoutSeconds, maxTimeoutSeconds)
	if err != nil {
		return nil, err
	}
	cfg.Timeout = time.Duration(seconds) * time.Second

	if cfg.Retries, err = parseIntInRange(input("retries"), "retries", 2, 0, maxRetries); err != nil {
		return nil, err
	}

	return cfg, nil
}

func parseRecipient(chatID, userID string) (string, int64, error) {
	switch {
	case chatID != "" && userID != "":
		return "", 0, errors.New("exactly one of chat_id or user_id must be provided")
	case chatID != "":
		id, err := parseID(chatID, "chat_id")
		return RecipientChat, id, err
	case userID != "":
		id, err := parseID(userID, "user_id")
		return RecipientUser, id, err
	default:
		return "", 0, errors.New("exactly one of chat_id or user_id must be provided")
	}
}

// parseID accepts any signed 64-bit integer: MAX chat IDs may be negative.
func parseID(value, name string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return id, nil
}

func parseBool(value, name string, fallback bool) (bool, error) {
	if value == "" {
		return fallback, nil
	}
	switch strings.ToLower(value) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", name)
	}
}

func parseIntInRange(value, name string, fallback, minValue, maxValue int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	if parsed < minValue || parsed > maxValue {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minValue, maxValue)
	}
	return parsed, nil
}

func parseBaseURL(value string) (*url.URL, error) {
	if value == "" {
		value = DefaultAPIBaseURL
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, errors.New("api_base_url must be a valid absolute URL")
	}
	if parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("api_base_url must be an absolute http or https URL")
	}
	if parsed.Scheme != "https" && isMAXHost(parsed.Hostname()) {
		return nil, errors.New("api_base_url must use https for MAX API hosts")
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return parsed, nil
}

// isMAXHost reports whether the host belongs to MAX, where plaintext HTTP would
// leak the Authorization header.
func isMAXHost(host string) bool {
	host = strings.ToLower(host)
	return host == "max.ru" || strings.HasSuffix(host, ".max.ru")
}
