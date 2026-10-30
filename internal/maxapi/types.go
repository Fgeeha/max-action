package maxapi

import "fmt"

// SendMessage is the request accepted by Send.
type SendMessage struct {
	RecipientType      string
	RecipientID        int64
	Text               string
	Format             string
	Notify             bool
	DisableLinkPreview bool
	Attachments        []Attachment
}

// Attachment is an uploaded file referenced by its upload token.
type Attachment struct {
	Type    string            `json:"type"`
	Payload AttachmentPayload `json:"payload"`
}

// AttachmentPayload carries the token returned by the upload host.
type AttachmentPayload struct {
	Token string `json:"token"`
}

// messageBody is the JSON body of POST /messages. Recipient and
// disable_link_preview travel as query parameters, per the MAX API.
type messageBody struct {
	Text        string       `json:"text"`
	Notify      bool         `json:"notify"`
	Format      string       `json:"format,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// sendResponse is the documented success payload. Unknown fields are ignored
// and every field is optional.
type sendResponse struct {
	Message struct {
		Body struct {
			MID string `json:"mid"`
		} `json:"body"`
	} `json:"message"`
}

// apiError is the documented MAX error payload.
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error reports a non-2xx response. Code is the MAX error code when present;
// Detail is already sanitized and truncated.
type Error struct {
	StatusCode int
	Code       string
	Detail     string
}

func (e *Error) Error() string {
	status := fmt.Sprintf("MAX API returned HTTP %d", e.StatusCode)
	// The code is what the README troubleshooting table matches on, and MAX
	// often words the message differently.
	if e.Code != "" && e.Code != e.Detail {
		status += " (" + e.Code + ")"
	}
	if e.Detail == "" {
		return status
	}
	return status + ": " + e.Detail
}
