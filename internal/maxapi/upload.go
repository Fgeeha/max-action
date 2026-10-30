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
	"time"
)

const (
	// MaxAttachmentBytes caps the file buffered for upload. MAX itself accepts
	// much larger files; the encoded body is kept in memory so a failed upload
	// can be retried, which is the ceiling here.
	MaxAttachmentBytes = 50 << 20

	// uploadTimeout bounds the whole upload — every attempt included — since
	// moving the file can take far longer than an ordinary API call.
	uploadTimeout = 5 * time.Minute

	// notReadyCode is returned by POST /messages while MAX is still processing
	// a freshly uploaded attachment.
	notReadyCode = "attachment.not.ready"
	// notReadyAttempts and notReadyDelay bound the wait for that processing:
	// about 20 seconds in total.
	notReadyAttempts = 10
	notReadyDelay    = 2 * time.Second
)

// UploadFile uploads a local file and returns the attachment to send with a
// message. The token is sent only to the MAX API: the upload host gets a
// pre-signed URL and never sees Authorization.
func (c *Client) UploadFile(ctx context.Context, path string) (Attachment, error) {
	ctx, cancel := context.WithTimeout(ctx, uploadTimeout)
	defer cancel()

	body, contentType, size, err := encodeAttachment(path)
	if err != nil {
		return Attachment{}, err
	}

	uploadURL, err := c.uploadURL(ctx)
	if err != nil {
		return Attachment{}, err
	}

	c.log(c.debugf, "uploading a %d-byte file to %s", size, uploadURL.Host)
	data, err := c.retrying(ctx, uploadTimeout, request{url: uploadURL, body: body, contentType: contentType})
	if err != nil {
		return Attachment{}, fmt.Errorf("cannot upload file: %w", err)
	}

	var parsed struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil || parsed.Token == "" {
		return Attachment{}, errors.New("cannot upload file: upload response has no token")
	}
	return Attachment{Type: "file", Payload: AttachmentPayload{Token: parsed.Token}}, nil
}

// uploadURL asks the MAX API where to put the file: POST /uploads?type=file.
func (c *Client) uploadURL(ctx context.Context) (*url.URL, error) {
	endpoint := c.apiURL("/uploads", url.Values{"type": {"file"}})
	data, err := c.retrying(ctx, c.timeout, request{url: endpoint, auth: true})
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

// encodeAttachment checks the file and writes it straight into a multipart
// body as field "data", so the file is held in memory only once.
func encodeAttachment(path string) (body []byte, contentType string, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", 0, fmt.Errorf("attachment_file cannot be read: %s", path)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, "", 0, fmt.Errorf("attachment_file is not a regular file: %s", path)
	}
	if info.Size() == 0 {
		return nil, "", 0, fmt.Errorf("attachment_file is empty: %s", path)
	}
	if info.Size() > MaxAttachmentBytes {
		return nil, "", 0, fmt.Errorf("attachment_file exceeds %d MiB: %s", MaxAttachmentBytes>>20, path)
	}

	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	part, err := form.CreateFormFile("data", filepath.Base(path))
	if err != nil {
		return nil, "", 0, fmt.Errorf("cannot encode upload: %w", err)
	}
	// The file may grow between Stat and read: copy one byte past the limit
	// to notice.
	n, err := io.Copy(part, io.LimitReader(f, MaxAttachmentBytes+1))
	if err != nil {
		return nil, "", 0, fmt.Errorf("attachment_file cannot be read: %s", path)
	}
	if n > MaxAttachmentBytes {
		return nil, "", 0, fmt.Errorf("attachment_file exceeds %d MiB: %s", MaxAttachmentBytes>>20, path)
	}
	if err := form.Close(); err != nil {
		return nil, "", 0, fmt.Errorf("cannot encode upload: %w", err)
	}
	return buf.Bytes(), form.FormDataContentType(), n, nil
}

// isNotReady reports whether MAX is still processing an attachment.
func isNotReady(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Code == notReadyCode
}
