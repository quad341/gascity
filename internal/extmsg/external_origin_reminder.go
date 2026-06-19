package extmsg

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gastownhall/gascity/internal/beadmeta"
)

// RenderExternalOriginSystemReminder returns an <external-origin> block for
// injection into agent system-reminder output when the active work bead carries
// an external-origin context. The block includes the conversation identifiers
// and the provider-neutral gc extmsg reply instruction.
//
// Returns (block, true) when gc.extmsg.origin is present and non-empty in
// metadata. Returns ("", false) otherwise.
func RenderExternalOriginSystemReminder(metadata map[string]string) (string, bool) {
	raw, ok := metadata[beadmeta.ExtMsgOriginMetadataKey]
	if !ok || raw == "" {
		return "", false
	}

	var env ExternalOriginEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return "", false
	}

	refJSON, err := json.Marshal(env.Conversation)
	if err != nil {
		return "", false
	}

	var sb strings.Builder
	sb.WriteString("<external-origin>\n")
	fmt.Fprintf(&sb, "provider: %s\n", env.Conversation.Provider)
	fmt.Fprintf(&sb, "account_id: %s\n", env.Conversation.AccountID)
	fmt.Fprintf(&sb, "conversation_id: %s\n", env.Conversation.ConversationID)
	fmt.Fprintf(&sb, "binding_id: %s\n", env.BindingID)
	sb.WriteString("Reply to this conversation using:\n")
	sb.WriteString("  gc extmsg reply \"your response here\"\n")
	sb.WriteString("Or, to include the ref explicitly:\n")
	fmt.Fprintf(&sb, "  gc extmsg reply --ref %s \"your response here\"\n", string(refJSON))
	sb.WriteString("</external-origin>\n")

	return sb.String(), true
}
