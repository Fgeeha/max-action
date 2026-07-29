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
}

// messageBody is the JSON body of POST /messages. Recipient and
// disable_link_preview travel as query parameters, per the MAX API.
type messageBody struct {
	Text   string `json:"text"`
	Notify bool   `json:"notify"`
	Format string `json:"format,omitempty"`
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

// Error reports a non-2xx response. Detail is already sanitized and truncated.
type Error struct {
	StatusCode int
	Detail     string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("MAX API returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("MAX API returned HTTP %d: %s", e.StatusCode, e.Detail)
}
