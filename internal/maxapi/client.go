// Package maxapi implements the minimal MAX Bot API client needed to send one
// text message.
package maxapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// maxBodyBytes caps how much of a response we are willing to read.
	maxBodyBytes = 1 << 20
	// maxDetailRunes caps the API error text surfaced to the workflow log.
	maxDetailRunes = 300
	// maxRetryAfter caps how long a Retry-After header can hold us.
	maxRetryAfter = 60 * time.Second

	backoffBase = 500 * time.Millisecond
	backoffMax  = 10 * time.Second
)

// errRedirectBlocked is returned instead of following a redirect, which would
// risk replaying the Authorization header against another host.
var errRedirectBlocked = errors.New("redirects are not allowed: the MAX API must be reached directly")

// Logf writes a diagnostic line. A nil Logf discards the message.
type Logf func(format string, args ...any)

// Options configures a Client.
type Options struct {
	BaseURL *url.URL
	Token   string
	Timeout time.Duration
	Retries int
	Version string
	Debugf  Logf
	Warnf   Logf
}

// Client sends messages through the MAX Bot API.
type Client struct {
	base      *url.URL
	token     string
	userAgent string
	retries   int
	timeout   time.Duration
	hc        *http.Client
	debugf    Logf
	warnf     Logf
}

// New builds a Client that trusts the system roots plus the MAX certificate
// chain, and never follows redirects.
func New(opts Options) (*Client, error) {
	roots, err := rootPool()
	if err != nil {
		return nil, err
	}

	version := opts.Version
	if version == "" {
		version = "dev"
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}

	return &Client{
		base:      opts.BaseURL,
		token:     opts.Token,
		userAgent: "Fgeeha/max-action/" + version,
		retries:   opts.Retries,
		timeout:   opts.Timeout,
		hc: &http.Client{
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errRedirectBlocked
			},
		},
		debugf: opts.Debugf,
		warnf:  opts.Warnf,
	}, nil
}

// Send posts a single message and returns the message ID when the API reports
// one. An empty ID with a nil error still means the message was accepted.
func (c *Client) Send(ctx context.Context, msg SendMessage) (string, error) {
	endpoint := c.endpoint(msg)
	body, err := json.Marshal(messageBody{Text: msg.Text, Notify: msg.Notify, Format: msg.Format})
	if err != nil {
		return "", fmt.Errorf("cannot encode request body: %w", err)
	}

	c.log(c.debugf, "sending to %s recipient via %s (%d characters)",
		msg.RecipientType, endpoint.Host, utf8.RuneCountInString(msg.Text))

	var lastErr error
	for attempt := 0; ; attempt++ {
		mid, hint, err := c.attempt(ctx, endpoint, body, attempt)
		if err == nil {
			return mid, nil
		}
		lastErr = err

		if !hint.retry || attempt >= c.retries {
			return "", lastErr
		}
		delay := hint.after
		if delay <= 0 {
			delay = backoff(attempt)
		}
		if !c.canWait(ctx, delay) {
			return "", lastErr
		}
		c.log(c.debugf, "attempt %d failed (%v), retrying in %s", attempt+1, err, delay.Round(time.Millisecond))
		if err := wait(ctx, delay); err != nil {
			return "", lastErr
		}
	}
}

// retryHint tells the caller whether and when to retry.
type retryHint struct {
	retry bool
	after time.Duration
}

func (c *Client) attempt(parent context.Context, endpoint *url.URL, body []byte, attempt int) (string, retryHint, error) {
	ctx, cancel := context.WithTimeout(parent, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return "", retryHint{}, fmt.Errorf("cannot build request: %w", err)
	}
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	started := time.Now()
	resp, err := c.hc.Do(req)
	if err != nil {
		if errors.Is(err, errRedirectBlocked) {
			return "", retryHint{}, errRedirectBlocked
		}
		// The caller giving up is not a transient failure.
		if parent.Err() != nil {
			return "", retryHint{}, fmt.Errorf("request aborted: %s", c.redact(err.Error()))
		}
		return "", retryHint{retry: true}, fmt.Errorf("request failed: %s", c.redact(err.Error()))
	}
	defer resp.Body.Close()

	c.log(c.debugf, "attempt %d: HTTP %d in %s", attempt+1, resp.StatusCode, time.Since(started).Round(time.Millisecond))

	data, err := readLimited(resp.Body)
	if err != nil {
		return "", retryHint{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", c.hintFor(resp), &Error{StatusCode: resp.StatusCode, Detail: c.errorDetail(data)}
	}
	return c.messageID(data), retryHint{}, nil
}

// messageID extracts message.body.mid, tolerating an empty body, missing
// fields and unknown fields. A body we cannot parse is reported as a warning:
// the API already accepted the message.
func (c *Client) messageID(data []byte) string {
	if len(bytes.TrimSpace(data)) == 0 {
		c.log(c.debugf, "MAX returned a success status with an empty body")
		return ""
	}
	var parsed sendResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		c.log(c.warnf, "message sent, but the MAX response could not be parsed; message_id is unavailable")
		return ""
	}
	if parsed.Message.Body.MID == "" {
		c.log(c.debugf, "MAX response contained no message ID")
	}
	return parsed.Message.Body.MID
}

// hintFor decides whether a non-2xx status is worth retrying.
func (c *Client) hintFor(resp *http.Response) retryHint {
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return retryHint{retry: true, after: retryAfter(resp.Header.Get("Retry-After"))}
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return retryHint{retry: true}
	default:
		// 400, 401, 403, 404 and every other status will not change on retry.
		return retryHint{}
	}
}

// endpoint builds POST /messages with the recipient and link-preview flag as
// query parameters. The token is never part of the URL.
func (c *Client) endpoint(msg SendMessage) *url.URL {
	endpoint := *c.base
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/messages"

	query := url.Values{}
	query.Set(msg.RecipientType+"_id", strconv.FormatInt(msg.RecipientID, 10))
	if msg.DisableLinkPreview {
		query.Set("disable_link_preview", "true")
	}
	endpoint.RawQuery = query.Encode()
	return &endpoint
}

// errorDetail pulls a human-readable reason out of an error response.
func (c *Client) errorDetail(data []byte) string {
	if len(bytes.TrimSpace(data)) == 0 {
		return ""
	}
	var parsed apiError
	if err := json.Unmarshal(data, &parsed); err == nil && (parsed.Message != "" || parsed.Code != "") {
		if parsed.Message != "" {
			return c.clean(parsed.Message)
		}
		return c.clean(parsed.Code)
	}
	return c.clean(string(data))
}

// clean strips control characters, collapses whitespace, redacts the token and
// truncates, so the text is safe for a workflow command.
func (c *Client) clean(s string) string {
	s = c.redact(s)
	// Control characters become spaces rather than being dropped, so removing
	// them cannot fuse two words into a misleading one.
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")

	if utf8.RuneCountInString(s) > maxDetailRunes {
		s = string([]rune(s)[:maxDetailRunes]) + "..."
	}
	return s
}

// redact removes the token from any text that might reach the log.
func (c *Client) redact(s string) string {
	if c.token == "" {
		return s
	}
	return strings.ReplaceAll(s, c.token, "***")
}

func (c *Client) log(fn Logf, format string, args ...any) {
	if fn != nil {
		fn(format, args...)
	}
}

// canWait reports whether the delay fits inside the caller's deadline.
func (c *Client) canWait(ctx context.Context, delay time.Duration) bool {
	deadline, ok := ctx.Deadline()
	return !ok || time.Until(deadline) > delay
}

func readLimited(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("cannot read MAX response: %w", err)
	}
	if len(data) > maxBodyBytes {
		return nil, fmt.Errorf("MAX response exceeds %d bytes", maxBodyBytes)
	}
	return data, nil
}

// retryAfter parses the Retry-After header in both documented forms, ignoring
// values that are absent, malformed or unreasonably long.
func retryAfter(header string) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(header); err == nil {
		return clampRetryAfter(time.Duration(seconds) * time.Second)
	}
	if at, err := http.ParseTime(header); err == nil {
		return clampRetryAfter(time.Until(at))
	}
	return 0
}

func clampRetryAfter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	return min(d, maxRetryAfter)
}

// backoff grows exponentially with jitter to avoid synchronised retries.
func backoff(attempt int) time.Duration {
	delay := min(backoffBase<<attempt, backoffMax)
	return delay + time.Duration(rand.Int63n(int64(delay/2)+1))
}

func wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
