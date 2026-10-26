package honk

import (
	"bytes"
	"encoding/json"
	"strings"
)

// The SDK does not check the contents of a message: the server corrects what it can (and says
// so in Accepted.Warnings) and refuses only what it cannot deliver. The only local checks are
// that a message has a title or a message, and that an idempotency key can be sent as a header.

// validIdempotencyKey: 1–128 printable ASCII characters (0x21–0x7E), so the header can be sent.
func validIdempotencyKey(k string) bool {
	if len(k) == 0 || len(k) > 128 {
		return false
	}
	for i := 0; i < len(k); i++ {
		if k[i] < 0x21 || k[i] > 0x7e {
			return false
		}
	}
	return true
}

// EncodeMessage returns the exact JSON body Honk would post for m (defaults applied). Useful
// for logging, dry runs and tests. It fails only when m has neither a title nor a message.
func EncodeMessage(m Message, defaults Defaults) ([]byte, error) {
	return encodeBody(m, defaults)
}

func encodeBody(m Message, d Defaults) ([]byte, error) {
	m = m.normalized(d)
	if strings.TrimSpace(m.Message) == "" && strings.TrimSpace(m.Title) == "" {
		return nil, localValidation([]FieldError{{Field: "message", Code: "required", Message: "a message or a title is required"}})
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m.wire()); err != nil {
		return nil, localValidation([]FieldError{{Field: "body", Code: "invalid_format", Message: err.Error()}})
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
