package extmsg

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SSEMessageEvent is the data payload for event: message.
// Emitted when the target session produces a reply for this conversation.
type SSEMessageEvent struct {
	Version      string          `json:"version"`
	Event        string          `json:"event"`
	Text         string          `json:"text"`
	SessionID    string          `json:"session_id"`
	Conversation ConversationRef `json:"conversation"`
	Sequence     int64           `json:"sequence"`
	CreatedAt    time.Time       `json:"created_at"`
}

// SSEHeartbeatEvent is the data payload for event: heartbeat.
// Emitted on idle intervals to signal the stream is alive. Does not
// advance the replay cursor (no id: field).
type SSEHeartbeatEvent struct {
	Version string    `json:"version"`
	Event   string    `json:"event"`
	TS      time.Time `json:"ts"`
}

// SSEErrorEvent is the data payload for event: error.
// Emitted when the server must close the stream. RetryAfterMs is present
// only when Retryable is true.
type SSEErrorEvent struct {
	Version      string `json:"version"`
	Event        string `json:"event"`
	Code         string `json:"code"`
	Message      string `json:"message"`
	Retryable    bool   `json:"retryable"`
	RetryAfterMs *int64 `json:"retry_after_ms,omitempty"`
}

// FormatConnectedClientSSEMessage formats a message event as an SSE frame.
// Sets id: to the decimal sequence, event: message, and data: to the JSON payload.
func FormatConnectedClientSSEMessage(event SSEMessageEvent) ([]byte, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("marshal SSE message event: %w", err)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "id: %d\n", event.Sequence)
	sb.WriteString("event: message\n")
	fmt.Fprintf(&sb, "data: %s\n", data)
	sb.WriteString("\n")
	return []byte(sb.String()), nil
}

// FormatConnectedClientSSEHeartbeat formats a heartbeat event as an SSE frame.
// Omits the id: field so the replay cursor is not advanced.
func FormatConnectedClientSSEHeartbeat(event SSEHeartbeatEvent) ([]byte, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("marshal SSE heartbeat event: %w", err)
	}
	var sb strings.Builder
	sb.WriteString("event: heartbeat\n")
	fmt.Fprintf(&sb, "data: %s\n", data)
	sb.WriteString("\n")
	return []byte(sb.String()), nil
}

// FormatConnectedClientSSEError formats an error event as an SSE frame.
// Sets id: to the literal sentinel "error", and for retryable errors prepends
// a retry: line before the event: line. Closes the frame with a blank line.
func FormatConnectedClientSSEError(event SSEErrorEvent) ([]byte, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("marshal SSE error event: %w", err)
	}
	var sb strings.Builder
	if event.Retryable && event.RetryAfterMs != nil {
		fmt.Fprintf(&sb, "retry: %d\n", *event.RetryAfterMs)
	} else if event.Retryable {
		// Retryable with no specific delay — emit retry: 0 per W3C SSE.
		sb.WriteString("retry: 0\n")
	}
	sb.WriteString("id: error\n")
	sb.WriteString("event: error\n")
	fmt.Fprintf(&sb, "data: %s\n", data)
	sb.WriteString("\n")
	return []byte(sb.String()), nil
}

// ParseConnectedClientLastEventID parses the Last-Event-ID header for the
// connected-client subscribe endpoint. Returns (seq, true) when the header
// holds a valid decimal sequence number. Returns (0, false) for absent,
// empty, the "error" sentinel, or any non-numeric value.
func ParseConnectedClientLastEventID(header string) (int64, bool) {
	if header == "" || header == "error" {
		return 0, false
	}
	seq, err := strconv.ParseInt(header, 10, 64)
	if err != nil {
		return 0, false
	}
	return seq, true
}
