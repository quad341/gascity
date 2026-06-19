package extmsg

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ConnectedClientProvider is the provider name used for connected clients
// that subscribe via SSE. The account_id is the client_id assigned at registration.
const ConnectedClientProvider = "llm-client"

// ConnectedClientRecord is a registered connected-client credential mapping.
type ConnectedClientRecord struct {
	ClientID  string
	Token     string
	CreatedAt time.Time
}

// ConnectedClientService manages connected-client registrations and active SSE
// subscribers. It is safe for concurrent use.
type ConnectedClientService struct {
	mu          sync.RWMutex
	clients     map[string]ConnectedClientRecord // credential → record
	byToken     map[string]string                // token → clientID
	subscribers map[string]chan struct{}         // accountID/conversationID → notification channel
}

// NewConnectedClientService creates an empty ConnectedClientService.
func NewConnectedClientService() *ConnectedClientService {
	return &ConnectedClientService{
		clients:     make(map[string]ConnectedClientRecord),
		byToken:     make(map[string]string),
		subscribers: make(map[string]chan struct{}),
	}
}

// Register returns (or creates) a ConnectedClientRecord for the given credential.
// The second return value is true when a new record was created.
func (s *ConnectedClientService) Register(credential string) (ConnectedClientRecord, bool, error) {
	if credential == "" {
		return ConnectedClientRecord{}, false, fmt.Errorf("%w: credential required", ErrInvalidInput)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec, ok := s.clients[credential]; ok {
		return rec, false, nil
	}
	clientID, err := generateConnectedClientID()
	if err != nil {
		return ConnectedClientRecord{}, false, fmt.Errorf("generate client_id: %w", err)
	}
	token, err := generateConnectedClientID()
	if err != nil {
		return ConnectedClientRecord{}, false, fmt.Errorf("generate token: %w", err)
	}
	rec := ConnectedClientRecord{
		ClientID:  clientID,
		Token:     token,
		CreatedAt: time.Now().UTC(),
	}
	s.clients[credential] = rec
	s.byToken[token] = clientID
	return rec, true, nil
}

// LookupByToken returns the clientID for the given token, or "" if not found.
func (s *ConnectedClientService) LookupByToken(token string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byToken[token]
}

// Subscribe registers a notification channel for the given (accountID, conversationID) pair.
// Returns a buffered channel that receives a struct{} when a new outbound message is available.
// The caller MUST call Unsubscribe when done.
func (s *ConnectedClientService) Subscribe(accountID, conversationID string) chan struct{} {
	key := accountID + "/" + conversationID
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	s.subscribers[key] = ch
	s.mu.Unlock()
	return ch
}

// Unsubscribe removes the active subscriber for (accountID, conversationID).
func (s *ConnectedClientService) Unsubscribe(accountID, conversationID string) {
	key := accountID + "/" + conversationID
	s.mu.Lock()
	delete(s.subscribers, key)
	s.mu.Unlock()
}

// HasSubscriber reports whether an active SSE subscriber exists for (accountID, conversationID).
func (s *ConnectedClientService) HasSubscriber(accountID, conversationID string) bool {
	key := accountID + "/" + conversationID
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.subscribers[key]
	return ok
}

// Notify signals the subscriber for (accountID, conversationID), if any.
// Returns true when an active subscriber was signaled, false otherwise.
// Call this AFTER the outbound transcript entry is written so the subscriber
// always finds the entry on the next transcript read (via OutboundDeps.PostTranscript).
func (s *ConnectedClientService) Notify(accountID, conversationID string) bool {
	key := accountID + "/" + conversationID
	s.mu.RLock()
	ch, ok := s.subscribers[key]
	s.mu.RUnlock()
	if !ok {
		return false
	}
	select {
	case ch <- struct{}{}:
	default:
		// Buffered channel is full — subscriber already has a pending notification.
	}
	return true
}

// ConnectedClientAdapter implements TransportAdapter for the llm-client provider.
// It delivers messages to active SSE subscribers via ConnectedClientService,
// returning Delivered=false when no subscriber is currently connected.
type ConnectedClientAdapter struct {
	accountID string
	svc       *ConnectedClientService
}

// NewConnectedClientAdapter creates an adapter for the given accountID.
func NewConnectedClientAdapter(accountID string, svc *ConnectedClientService) *ConnectedClientAdapter {
	return &ConnectedClientAdapter{accountID: accountID, svc: svc}
}

// Name returns the adapter identifier.
func (a *ConnectedClientAdapter) Name() string { return ConnectedClientProvider + "/" + a.accountID }

// Capabilities returns the adapter capabilities.
func (a *ConnectedClientAdapter) Capabilities() AdapterCapabilities {
	return AdapterCapabilities{}
}

// VerifyAndNormalizeInbound is not used by the connected-client adapter;
// connected clients send pre-normalized messages via the inbound endpoint.
func (a *ConnectedClientAdapter) VerifyAndNormalizeInbound(_ context.Context, _ InboundPayload) (*ExternalInboundMessage, error) {
	return nil, errors.New("connected client uses the normalized inbound path")
}

// Publish reports whether a subscriber is connected for the target conversation.
// It does NOT signal the subscriber — that happens via OutboundDeps.PostTranscript
// in HandleOutbound, after the transcript entry is written, to guarantee that the
// subscriber always finds the entry when it wakes up.
func (a *ConnectedClientAdapter) Publish(_ context.Context, req PublishRequest) (*PublishReceipt, error) {
	has := a.svc.HasSubscriber(a.accountID, req.Conversation.ConversationID)
	receipt := &PublishReceipt{
		Conversation: req.Conversation,
		Delivered:    has,
	}
	if !has {
		receipt.FailureKind = PublishFailureNoSubscriber
	}
	return receipt, nil
}

// EnsureChildConversation is not supported by the connected-client adapter.
func (a *ConnectedClientAdapter) EnsureChildConversation(_ context.Context, _ ConversationRef, _ string) (*ConversationRef, error) {
	return nil, errors.New("connected client does not support child conversations")
}

func generateConnectedClientID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
