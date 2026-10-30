package maxapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// uploadServer imitates POST /uploads, the upload host and POST /messages on
// one test server. It records what reached the upload host.
type uploadServer struct {
	*httptest.Server
	uploadAuth  string
	uploadName  string
	uploadData  string
	messageBody []byte
	notReady    int32
	sends       int32
}

func newUploadServer(t *testing.T, notReady int32) *uploadServer {
	t.Helper()
	s := &uploadServer{notReady: notReady}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/uploads":
			if r.Header.Get("Authorization") != testToken || r.URL.Query().Get("type") != "file" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			io.WriteString(w, `{"url":"`+s.URL+`/upload-host?sig=abc"}`)
		case "/upload-host":
			s.uploadAuth = r.Header.Get("Authorization")
			file, header, err := r.FormFile("data")
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			data, _ := io.ReadAll(file)
			s.uploadName, s.uploadData = header.Filename, string(data)
			io.WriteString(w, `{"fileId":1,"token":"file-token"}`)
		case "/messages":
			n := atomic.AddInt32(&s.sends, 1)
			s.messageBody, _ = io.ReadAll(r.Body)
			if n <= atomic.LoadInt32(&s.notReady) {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, `{"code":"attachment.not.ready","message":"Key: errors.process.attachment.file.not.processed"}`)
				return
			}
			okResponse(w)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return path
}

func TestUploadFileNeverSendsTokenToUploadHost(t *testing.T) {
	server := newUploadServer(t, 0)
	client := newTestClient(t, server.URL, 0)

	attachment, err := client.UploadFile(context.Background(), writeFile(t, "docs.zip", "zip-bytes"))
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	if attachment != (Attachment{Type: "file", Payload: AttachmentPayload{Token: "file-token"}}) {
		t.Fatalf("attachment = %+v", attachment)
	}
	if server.uploadAuth != "" {
		t.Fatal("Authorization header reached the upload host")
	}
	if server.uploadName != "docs.zip" || server.uploadData != "zip-bytes" {
		t.Fatalf("upload host got %q with %q", server.uploadName, server.uploadData)
	}
}

func TestSendWaitsForAttachmentProcessing(t *testing.T) {
	server := newUploadServer(t, 2)
	client := newTestClient(t, server.URL, 0)
	client.notReadyDelay = time.Millisecond

	msg := chatMessage()
	msg.Attachments = []Attachment{{Type: "file", Payload: AttachmentPayload{Token: "file-token"}}}
	mid, err := client.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if mid != "mid-1" || atomic.LoadInt32(&server.sends) != 3 {
		t.Fatalf("mid=%q sends=%d, want mid-1 after 3 sends", mid, server.sends)
	}

	var body struct {
		Attachments []Attachment `json:"attachments"`
	}
	if err := json.Unmarshal(server.messageBody, &body); err != nil {
		t.Fatalf("message body: %v", err)
	}
	if len(body.Attachments) != 1 || body.Attachments[0].Payload.Token != "file-token" {
		t.Fatalf("attachments in body = %+v", body.Attachments)
	}
}

func TestSendGivesUpWhenAttachmentNeverReady(t *testing.T) {
	server := newUploadServer(t, 100)
	client := newTestClient(t, server.URL, 0)
	client.notReadyDelay = time.Millisecond

	_, err := client.Send(context.Background(), chatMessage())
	if !isNotReady(err) {
		t.Fatalf("err = %v, want attachment.not.ready", err)
	}
	if got := atomic.LoadInt32(&server.sends); got != notReadyAttempts {
		t.Fatalf("sends = %d, want %d", got, notReadyAttempts)
	}
}

func TestUploadRejectsUnsafeUploadURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"url":"ftp://files.example/upload"}`)
	}))
	defer server.Close()

	_, err := newTestClient(t, server.URL, 0).UploadFile(context.Background(), writeFile(t, "a.txt", "x"))
	if err == nil || !strings.Contains(err.Error(), "non-https") {
		t.Fatalf("err = %v, want non-https rejection", err)
	}
}

func TestReadAttachmentLimits(t *testing.T) {
	if _, err := readAttachment(writeFile(t, "empty.txt", "")); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty file: err = %v", err)
	}
	if _, err := readAttachment(t.TempDir()); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("directory: err = %v", err)
	}

	big := filepath.Join(t.TempDir(), "big.bin")
	f, err := os.Create(big)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Sparse: no real 50 MiB write.
	if err := f.Truncate(MaxAttachmentBytes + 1); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	f.Close()
	if _, err := readAttachment(big); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversize: err = %v", err)
	}
}
