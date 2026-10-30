package maxapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testToken = "super-secret-token"

// newTestClient points a client at a test server with fast retries.
func newTestClient(t *testing.T, serverURL string, retries int) *Client {
	t.Helper()
	base, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	client, err := New(Options{
		BaseURL: base,
		Token:   testToken,
		Timeout: 2 * time.Second,
		Retries: retries,
		Version: "test",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return client
}

func chatMessage() SendMessage {
	return SendMessage{RecipientType: "chat", RecipientID: 123, Text: "hello", Notify: true}
}

func okResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, `{"message":{"body":{"mid":"mid-1"},"seq":7},"unknown":true}`)
}

func TestSendRequestShape(t *testing.T) {
	var got *http.Request
	var body []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		body, _ = io.ReadAll(r.Body)
		okResponse(w)
	}))
	defer server.Close()

	msg := chatMessage()
	msg.Format = "markdown"
	mid, err := newTestClient(t, server.URL, 0).Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mid != "mid-1" {
		t.Errorf("mid = %q, want mid-1", mid)
	}

	if got.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", got.Method)
	}
	if got.URL.Path != "/messages" {
		t.Errorf("path = %s, want /messages", got.URL.Path)
	}
	if v := got.URL.Query().Get("chat_id"); v != "123" {
		t.Errorf("chat_id = %q, want 123", v)
	}
	if got.URL.Query().Has("disable_link_preview") {
		t.Error("disable_link_preview should be absent when false")
	}
	if got.Header.Get("Authorization") != testToken {
		t.Errorf("Authorization header = %q", got.Header.Get("Authorization"))
	}
	if got.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", got.Header.Get("Content-Type"))
	}
	if ua := got.Header.Get("User-Agent"); ua != "Fgeeha/max-action/test" {
		t.Errorf("User-Agent = %q", ua)
	}
	if strings.Contains(got.URL.String(), testToken) {
		t.Error("token leaked into the URL")
	}

	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	want := map[string]any{"text": "hello", "notify": true, "format": "markdown"}
	for k, v := range want {
		if decoded[k] != v {
			t.Errorf("body[%q] = %v, want %v", k, decoded[k], v)
		}
	}
	if len(decoded) != len(want) {
		t.Errorf("unexpected body fields: %v", decoded)
	}
}

func TestSendQueryVariants(t *testing.T) {
	tests := []struct {
		name      string
		msg       SendMessage
		wantQuery map[string]string
		absent    []string
	}{
		{
			name:      "user recipient",
			msg:       SendMessage{RecipientType: "user", RecipientID: 456, Text: "hi"},
			wantQuery: map[string]string{"user_id": "456"},
			absent:    []string{"chat_id", "disable_link_preview"},
		},
		{
			name:      "negative chat id",
			msg:       SendMessage{RecipientType: "chat", RecipientID: -1001, Text: "hi"},
			wantQuery: map[string]string{"chat_id": "-1001"},
		},
		{
			name:      "link preview disabled",
			msg:       SendMessage{RecipientType: "chat", RecipientID: 1, Text: "hi", DisableLinkPreview: true},
			wantQuery: map[string]string{"chat_id": "1", "disable_link_preview": "true"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var query url.Values
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query = r.URL.Query()
				okResponse(w)
			}))
			defer server.Close()

			if _, err := newTestClient(t, server.URL, 0).Send(context.Background(), tt.msg); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for k, v := range tt.wantQuery {
				if query.Get(k) != v {
					t.Errorf("query %s = %q, want %q", k, query.Get(k), v)
				}
			}
			for _, k := range tt.absent {
				if query.Has(k) {
					t.Errorf("query %s should be absent", k)
				}
			}
		})
	}
}

func TestSendOmitsEmptyFormat(t *testing.T) {
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		okResponse(w)
	}))
	defer server.Close()

	if _, err := newTestClient(t, server.URL, 0).Send(context.Background(), chatMessage()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(body), "format") {
		t.Errorf("empty format should be omitted, body = %s", body)
	}
}

func TestSendBaseURLWithPathPrefix(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		okResponse(w)
	}))
	defer server.Close()

	if _, err := newTestClient(t, server.URL+"/api/", 0).Send(context.Background(), chatMessage()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/api/messages" {
		t.Errorf("path = %s, want /api/messages", path)
	}
}

func TestSendSuccessResponses(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantMID string
	}{
		{"200 with mid", http.StatusOK, `{"message":{"body":{"mid":"mid-1"}}}`, "mid-1"},
		{"201 accepted", http.StatusCreated, `{"message":{"body":{"mid":"mid-2"}}}`, "mid-2"},
		{"204 empty body", http.StatusNoContent, "", ""},
		{"200 without mid", http.StatusOK, `{"message":{"body":{}}}`, ""},
		{"200 without message", http.StatusOK, `{}`, ""},
		{"200 with unknown fields", http.StatusOK, `{"message":{"body":{"mid":"m3","extra":1},"x":2},"y":3}`, "m3"},
		{"200 malformed JSON does not fail the send", http.StatusOK, `{not json`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				io.WriteString(w, tt.body)
			}))
			defer server.Close()

			mid, err := newTestClient(t, server.URL, 0).Send(context.Background(), chatMessage())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mid != tt.wantMID {
				t.Errorf("mid = %q, want %q", mid, tt.wantMID)
			}
		})
	}
}

func TestSendMalformedJSONWarns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{not json`)
	}))
	defer server.Close()

	base, _ := url.Parse(server.URL)
	var warnings []string
	client, err := New(Options{
		BaseURL: base, Token: testToken, Timeout: time.Second, Version: "test",
		Warnf: func(format string, args ...any) { warnings = append(warnings, format) },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Send(context.Background(), chatMessage()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 1 {
		t.Errorf("expected one warning, got %v", warnings)
	}
}

func TestSendNoRetryOnClientErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
				io.WriteString(w, `{"code":"bad.request","message":"invalid recipient"}`)
			}))
			defer server.Close()

			_, err := newTestClient(t, server.URL, 3).Send(context.Background(), chatMessage())
			if err == nil {
				t.Fatal("expected an error")
			}
			if calls.Load() != 1 {
				t.Errorf("made %d requests, want exactly 1", calls.Load())
			}

			var apiErr *Error
			if !asError(err, &apiErr) {
				t.Fatalf("error type = %T, want *maxapi.Error", err)
			}
			if apiErr.StatusCode != status {
				t.Errorf("status = %d, want %d", apiErr.StatusCode, status)
			}
			if !strings.Contains(err.Error(), "invalid recipient") {
				t.Errorf("error should carry the API message, got %q", err)
			}
		})
	}
}

func TestSendRetriesOnServerErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) < 3 {
					w.WriteHeader(status)
					return
				}
				okResponse(w)
			}))
			defer server.Close()

			mid, err := newTestClient(t, server.URL, 2).Send(context.Background(), chatMessage())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mid != "mid-1" {
				t.Errorf("mid = %q", mid)
			}
			if calls.Load() != 3 {
				t.Errorf("made %d requests, want 3", calls.Load())
			}
		})
	}
}

func TestSendStopsAfterRetryBudget(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if _, err := newTestClient(t, server.URL, 1).Send(context.Background(), chatMessage()); err == nil {
		t.Fatal("expected an error")
	}
	if calls.Load() != 2 {
		t.Errorf("made %d requests, want 2 (initial + 1 retry)", calls.Load())
	}
}

func TestSendHonoursRetryAfter(t *testing.T) {
	var calls atomic.Int32
	var gap time.Duration
	var first time.Time

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			first = time.Now()
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		gap = time.Since(first)
		okResponse(w)
	}))
	defer server.Close()

	base, _ := url.Parse(server.URL)
	client, err := New(Options{BaseURL: base, Token: testToken, Timeout: 3 * time.Second, Retries: 1, Version: "test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Send(context.Background(), chatMessage()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gap < time.Second {
		t.Errorf("waited %s, want at least the 1s from Retry-After", gap)
	}
}

func TestRetryAfterParsing(t *testing.T) {
	tests := []struct {
		header string
		want   time.Duration
	}{
		{"", 0},
		{"2", 2 * time.Second},
		{" 3 ", 3 * time.Second},
		{"soon", 0},
		{"-5", 0},
		{"99999", maxRetryAfter},
		{time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat), 0},
	}
	for _, tt := range tests {
		if got := retryAfter(tt.header); got != tt.want {
			t.Errorf("retryAfter(%q) = %s, want %s", tt.header, got, tt.want)
		}
	}
}

func TestSendRetriesNetworkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	serverURL := server.URL
	server.Close() // nothing is listening, so every attempt fails at the socket

	client := newTestClient(t, serverURL, 1)
	if _, err := client.Send(context.Background(), chatMessage()); err == nil {
		t.Fatal("expected an error")
	} else if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("error = %q", err)
	}
}

func TestSendTimesOut(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer func() {
		close(release)
		server.Close()
	}()

	base, _ := url.Parse(server.URL)
	client, err := New(Options{BaseURL: base, Token: testToken, Timeout: 100 * time.Millisecond, Version: "test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := client.Send(context.Background(), chatMessage()); err == nil {
		t.Fatal("expected a timeout error")
	}
}

func TestSendRejectsRedirects(t *testing.T) {
	var calls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the redirect target must never receive the Authorization header")
	}))
	defer target.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Redirect(w, r, target.URL+"/messages", http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL, 3).Send(context.Background(), chatMessage())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "redirects are not allowed") {
		t.Errorf("error = %q", err)
	}
	if calls.Load() != 1 {
		t.Errorf("made %d requests, want 1 (a blocked redirect is not retryable)", calls.Load())
	}
}

// A 2xx whose body cannot be read still means MAX accepted the message: the
// step must succeed without a message_id and must not send a second copy.
func TestSendToleratesUnreadableSuccessBody(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"oversized body", func(w http.ResponseWriter, r *http.Request) {
			w.Write(make([]byte, maxBodyBytes+1024))
		}},
		{"truncated body", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, `{"message":`)
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				tt.handler(w, r)
			}))
			defer server.Close()

			base, _ := url.Parse(server.URL)
			var warnings []string
			client, err := New(Options{
				BaseURL: base, Token: testToken, Timeout: 2 * time.Second, Retries: 3, Version: "test",
				Warnf: func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) },
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			mid, err := client.Send(context.Background(), chatMessage())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mid != "" {
				t.Errorf("mid = %q, want empty", mid)
			}
			if calls.Load() != 1 {
				t.Errorf("made %d requests, want 1: an accepted message must not be resent", calls.Load())
			}
			if len(warnings) != 1 || !strings.Contains(warnings[0], "discarded") {
				t.Errorf("warnings = %q, want one about the unreadable body", warnings)
			}
		})
	}
}

// When MAX accepts the POST but the response is lost on the way back, the
// client cannot tell this from a request that never arrived and retries it:
// the recipient gets the message twice. README documents this trade-off.
func TestSendRetryAfterLostResponseDuplicates(t *testing.T) {
	var delivered atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		if delivered.Add(1) == 1 {
			panic(http.ErrAbortHandler) // accepted, but the connection drops before a reply
		}
		okResponse(w)
	}))
	defer server.Close()

	mid, err := newTestClient(t, server.URL, 1).Send(context.Background(), chatMessage())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mid != "mid-1" {
		t.Errorf("mid = %q", mid)
	}
	if delivered.Load() != 2 {
		t.Errorf("server received %d requests, want 2", delivered.Load())
	}

	delivered.Store(0)
	if _, err := newTestClient(t, server.URL, 0).Send(context.Background(), chatMessage()); err == nil {
		t.Fatal("with retries 0 a lost response must surface as an error")
	}
	if delivered.Load() != 1 {
		t.Errorf("with retries 0 server received %d requests, want 1", delivered.Load())
	}
}

func TestErrorNeverLeaksToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"code":"auth","message":"token `+testToken+` is invalid"}`)
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL, 0).Send(context.Background(), chatMessage())
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("error leaked the token: %q", err)
	}
	if !strings.Contains(err.Error(), "***") {
		t.Errorf("token should be redacted, got %q", err)
	}
}

func TestErrorDetailIsSanitized(t *testing.T) {
	long := strings.Repeat("é", maxDetailRunes+50)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "plain text error\nwith\x00control\tchars "+long)
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL, 0).Send(context.Background(), chatMessage())
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if strings.ContainsAny(msg, "\n\r\x00") {
		t.Errorf("error still contains control characters: %q", msg)
	}
	if !strings.Contains(msg, "plain text error with control chars") {
		t.Errorf("error = %q", msg)
	}
	if !strings.HasSuffix(msg, "...") {
		t.Errorf("long error should be truncated, got %q", msg)
	}
}

func TestErrorWithEmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL, 0).Send(context.Background(), chatMessage())
	if err == nil || err.Error() != "MAX API returned HTTP 400" {
		t.Fatalf("error = %v", err)
	}
}

func TestSendStopsWhenContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := newTestClient(t, server.URL, 5).Send(ctx, chatMessage()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestEmbeddedCAIsLoadable(t *testing.T) {
	pool, err := rootPool()
	if err != nil {
		t.Fatalf("rootPool: %v", err)
	}
	if pool == nil {
		t.Fatal("pool is nil")
	}
}

// asError is errors.As, kept local to avoid importing errors in every test.
func asError(err error, target **Error) bool {
	for err != nil {
		if e, ok := err.(*Error); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
