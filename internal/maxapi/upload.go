package maxapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// MaxAttachmentBytes caps the file read into memory for upload. MAX itself
	// accepts much larger files; the file is buffered so a failed upload can be
	// retried, which is the ceiling here.
	MaxAttachmentBytes = 50 << 20

	// uploadTimeout bounds the transfer of the file itself, which can take far
	// longer than an ordinary API call.
	uploadTimeout = 5 * time.Minute

	// notReadyCode is returned by POST /messages while MAX is still processing
	// a freshly uploaded attachment.
	notReadyCode = "attachment.not.ready"
	// notReadyAttempts and notReadyDelay bound the wait for that processing.
	notReadyAttempts = 5
	notReadyDelay    = 2 * time.Second
)

// UploadFile uploads a local file and returns the attachment to send with a
// message. The token is sent only to the MAX API: the upload host gets a
// pre-signed URL and never sees Authorization.
func (c *Client) UploadFile(ctx context.Context, path string) (Attachment, error) {
	name := filepath.Base(path)
	data, err := readAttachment(path)
	if err != nil {
		return Attachment{}, err
	}

	uploadURL, err := c.uploadURL(ctx)
	if err != nil {
		return Attachment{}, err
	}

	c.log(c.debugf, "uploading %s (%d bytes) to %s", name, len(data), uploadURL.Host)
	token, err := c.uploadData(ctx, uploadURL, name, data)
	if err != nil {
		return Attachment{}, err
	}
	return Attachment{Type: "file", Payload: AttachmentPayload{Token: token}}, nil
}

// uploadURL asks the MAX API where to put the file: POST /uploads?type=file.
func (c *Client) uploadURL(ctx context.Context) (*url.URL, error) {
	endpoint := *c.base
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/uploads"
	endpoint.RawQuery = url.Values{"type": {"file"}}.Encode()

	data, err := c.retrying(ctx, func(attempt int) ([]byte, retryHint, error) {
		return c.do(ctx, c.timeout, request{url: &endpoint, auth: true}, attempt)
	})
	if err != nil {
		return nil, fmt.Errorf("cannot get upload URL: %w", err)
	}

	var parsed struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil || parsed.URL == "" {
		return nil, errors.New("cannot get upload URL: MAX response has no url")
	}
	target, err := url.Parse(parsed.URL)
	if err != nil || target.Host == "" {
		return nil, errors.New("cannot get upload URL: MAX returned an invalid url")
	}
	// The file must not travel in plaintext. A plain-http API base is only
	// ever a local test server, so the same scheme is allowed there.
	if target.Scheme != "https" && target.Scheme != c.base.Scheme {
		return nil, errors.New("cannot get upload URL: MAX returned a non-https url")
	}
	return target, nil
}

// uploadData posts the file as multipart field "data" and returns its token.
func (c *Client) uploadData(ctx context.Context, target *url.URL, name string, data []byte) (string, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("data", name)
	if err != nil {
		return "", fmt.Errorf("cannot encode upload: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("cannot encode upload: %w", err)
	}
	if err := form.Close(); err != nil {
		return "", fmt.Errorf("cannot encode upload: %w", err)
	}

	resp, err := c.retrying(ctx, func(attempt int) ([]byte, retryHint, error) {
		return c.do(ctx, uploadTimeout, request{
			url:         target,
			body:        body.Bytes(),
			contentType: form.FormDataContentType(),
		}, attempt)
	})
	if err != nil {
		return "", fmt.Errorf("cannot upload file: %w", err)
	}

	var parsed struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil || parsed.Token == "" {
		return "", errors.New("cannot upload file: upload response has no token")
	}
	return parsed.Token, nil
}

// readAttachment reads a regular file no larger than MaxAttachmentBytes.
func readAttachment(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("attachment_file cannot be read: %s", path)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("attachment_file is not a regular file: %s", path)
	}
	if info.Size() == 0 {
		return nil, fmt.Errorf("attachment_file is empty: %s", path)
	}
	if info.Size() > MaxAttachmentBytes {
		return nil, fmt.Errorf("attachment_file exceeds %d MiB: %s", MaxAttachmentBytes>>20, path)
	}

	data, err := io.ReadAll(io.LimitReader(f, MaxAttachmentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("attachment_file cannot be read: %s", path)
	}
	if len(data) > MaxAttachmentBytes {
		return nil, fmt.Errorf("attachment_file exceeds %d MiB: %s", MaxAttachmentBytes>>20, path)
	}
	return data, nil
}

// isNotReady reports whether MAX is still processing an attachment.
func isNotReady(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Code == notReadyCode
}
