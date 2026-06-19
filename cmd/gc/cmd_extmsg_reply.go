package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gastownhall/gascity/internal/api"
	"github.com/spf13/cobra"
)

// newExtmsgCmd returns the top-level "extmsg" command group.
func newExtmsgCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extmsg",
		Short: "External messaging operations",
	}
	cmd.AddCommand(newExtmsgReplyCmd(stdout, stderr))
	return cmd
}

// newExtmsgReplyCmd returns the "extmsg reply" subcommand.
func newExtmsgReplyCmd(stdout, stderr io.Writer) *cobra.Command {
	var (
		refFlag     string
		sessionFlag string
		dryRun      bool
	)

	cmd := &cobra.Command{
		Use:   "reply [text]",
		Short: "Reply to an external conversation",
		Long: `Send a reply to an external conversation identified by --ref.

Text is read from the first positional argument, or from stdin when no
argument is provided. Exit codes:

  0  delivered
  1  not delivered (subscriber rejected)
  2  no external-origin context available; use --ref
  3  session not found or not running`,
		RunE: func(_ *cobra.Command, args []string) error {
			cityPath, err := resolveCity()
			if err != nil {
				fmt.Fprintf(stderr, "gc: extmsg reply: %v\n", err) //nolint:errcheck // best-effort stderr
				return exitForCode(1)
			}

			// Resolve text: positional arg or stdin.
			var text string
			if len(args) > 0 {
				text = args[0]
			} else {
				data, readErr := io.ReadAll(os.Stdin)
				if readErr != nil {
					fmt.Fprintf(stderr, "gc: extmsg reply: read stdin: %v\n", readErr) //nolint:errcheck // best-effort stderr
					return exitForCode(1)
				}
				text = string(data)
			}

			// Resolve ConversationRef.
			if refFlag == "" {
				fmt.Fprintf(stderr, "gc: no active external-origin context; use --ref\n") //nolint:errcheck // best-effort stderr
				return exitForCode(2)
			}
			var conv extmsgConvRef
			if err := json.Unmarshal([]byte(refFlag), &conv); err != nil {
				fmt.Fprintf(stderr, "gc: extmsg reply: parse --ref: %v\n", err) //nolint:errcheck // best-effort stderr
				return exitForCode(1)
			}

			// Resolve session name.
			sessionName := sessionFlag
			if sessionName == "" {
				sessionName = os.Getenv("GC_SESSION_NAME")
			}

			// Get API client.
			c := apiClient(cityPath)
			if c == nil {
				fmt.Fprintf(stderr, "gc: extmsg reply: no API connection available\n") //nolint:errcheck // best-effort stderr
				return exitForCode(1)
			}

			// Verify session exists and is running.
			sessions, err := c.ListSessions("", "", false)
			if err != nil || !extmsgSessionRunning(sessions.Body, sessionName) {
				fmt.Fprintf(stderr, "gc: session %s not found or not running\n", sessionName) //nolint:errcheck // best-effort stderr
				return exitForCode(3)
			}

			if dryRun {
				refJSON, _ := json.Marshal(conv)
				fmt.Fprintf(stdout, "dry-run ref=%s session=%s text=%q\n", refJSON, sessionName, text) //nolint:errcheck // best-effort stdout
				return nil
			}

			// Post outbound.
			result, err := c.PostExtmsgOutbound(api.ExtmsgOutboundRequest{
				SessionID:      sessionName,
				Provider:       conv.Provider,
				AccountID:      conv.AccountID,
				ConversationID: conv.ConversationID,
				ScopeID:        conv.ScopeID,
				Kind:           conv.Kind,
				Text:           text,
			})
			if err != nil {
				fmt.Fprintf(stderr, "gc: extmsg reply: %v\n", err) //nolint:errcheck // best-effort stderr
				return exitForCode(1)
			}

			convKey := strings.Join([]string{result.Provider, result.AccountID, result.ConversationID}, "/")

			if result.Delivered {
				fmt.Fprintf(stdout, "delivered conversation=%s sequence=%d\n", convKey, result.Sequence) //nolint:errcheck // best-effort stdout
				return nil
			}

			reason := result.FailureKind
			if reason == "" {
				reason = "no_subscriber"
			}
			fmt.Fprintf(stdout, "not-delivered conversation=%s reason=%s\n", convKey, reason) //nolint:errcheck // best-effort stdout
			fmt.Fprintf(stderr, "gc: extmsg reply: not delivered (%s)\n", reason)             //nolint:errcheck // best-effort stderr
			return exitForCode(1)
		},
	}

	cmd.Flags().StringVar(&refFlag, "ref", "", "conversation ref JSON (overrides active-bead context)")
	cmd.Flags().StringVar(&sessionFlag, "session", "", "session ID to send from (default: $GC_SESSION_NAME)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the resolved request without sending")

	return cmd
}

// extmsgConvRef is the JSON shape of a conversation reference in --ref flags.
// Field names match the snake_case wire encoding used by ConversationRef.
type extmsgConvRef struct {
	Provider       string `json:"provider"`
	AccountID      string `json:"account_id"`
	ConversationID string `json:"conversation_id"`
	ScopeID        string `json:"scope_id"`
	Kind           string `json:"kind"`
}

// extmsgSessionRunning returns true when sessions contains an entry matching
// name by ID or session_name, and that entry is marked running.
func extmsgSessionRunning(sessions []api.SessionView, name string) bool {
	for _, s := range sessions {
		if s.ID == name || s.SessionName == name {
			return s.Running
		}
	}
	return false
}
