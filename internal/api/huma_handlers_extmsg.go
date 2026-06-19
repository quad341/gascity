package api

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/gastownhall/gascity/internal/events"
	"github.com/gastownhall/gascity/internal/extmsg"
)

// --- Huma helpers for extmsg ---

// humaExtmsgServices returns the extmsg services from state, returning an error
// if unavailable.
func (s *Server) humaExtmsgServices() (*extmsg.Services, error) {
	svc := s.state.ExtMsgServices()
	if svc == nil {
		return nil, huma.Error503ServiceUnavailable("external messaging not enabled")
	}
	return svc, nil
}

// humaExtmsgAdapterRegistry returns the adapter registry from state, returning
// an error if unavailable.
func (s *Server) humaExtmsgAdapterRegistry() (*extmsg.AdapterRegistry, error) {
	reg := s.state.AdapterRegistry()
	if reg == nil {
		return nil, huma.Error503ServiceUnavailable("adapter registry not available")
	}
	return reg, nil
}

// --- Inbound ---

// humaHandleExtMsgInbound is the Huma-typed handler for POST /v0/extmsg/inbound.
func (s *Server) humaHandleExtMsgInbound(ctx context.Context, input *ExtMsgInboundInput) (*ExtMsgInboundOutput, error) {
	svc, err := s.humaExtmsgServices()
	if err != nil {
		return nil, err
	}
	reg, err := s.humaExtmsgAdapterRegistry()
	if err != nil {
		return nil, err
	}

	deps := extmsg.InboundDeps{
		Services:  *svc,
		Registry:  reg,
		EmitEvent: s.extmsgEmitEvent(),
	}

	// Pre-normalized path.
	if input.Body.Message != nil {
		result, handleErr := extmsg.HandleInboundNormalized(ctx, deps, *input.Body.Message)
		if handleErr != nil {
			return nil, huma.Error422UnprocessableEntity(handleErr.Error())
		}
		go s.extmsgNotifyInboundMembers(s.backgroundCtx(), *input.Body.Message)
		out := &ExtMsgInboundOutput{}
		if result != nil {
			out.Body = *result
		}
		return out, nil
	}

	// Raw payload path. Provider and AccountID are only required when
	// Message is nil (the branch above handles the normalized case), so
	// the check stays here rather than in the schema — the schema can't
	// express conditional-on-sibling requiredness cleanly.
	if input.Body.Provider == "" || input.Body.AccountID == "" {
		return nil, huma.Error400BadRequest("provider and account_id are required for raw payloads")
	}

	key := extmsg.AdapterKey{Provider: input.Body.Provider, AccountID: input.Body.AccountID}
	result, err := extmsg.HandleInbound(ctx, deps, key, extmsg.InboundPayload{
		Body:       input.Body.Payload,
		ReceivedAt: time.Now(),
	})
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	out := &ExtMsgInboundOutput{}
	if result != nil {
		out.Body = *result
	}
	return out, nil
}

// --- Outbound ---

// humaHandleExtMsgOutbound is the Huma-typed handler for POST /v0/extmsg/outbound.
func (s *Server) humaHandleExtMsgOutbound(ctx context.Context, input *ExtMsgOutboundInput) (*ExtMsgOutboundOutput, error) {
	svc, err := s.humaExtmsgServices()
	if err != nil {
		return nil, err
	}
	reg, err := s.humaExtmsgAdapterRegistry()
	if err != nil {
		return nil, err
	}

	caller := extmsg.Caller{Kind: extmsg.CallerController, ID: "api"}
	deps := extmsg.OutboundDeps{
		Services:  *svc,
		Registry:  reg,
		EmitEvent: s.extmsgEmitEvent(),
		// PostTranscript notifies connected-client subscribers after the transcript
		// entry is committed, ensuring subscribers always find the entry on the next read.
		PostTranscript: func(conversation extmsg.ConversationRef, _ int64) {
			if svc.ConnectedClients != nil {
				svc.ConnectedClients.Notify(conversation.AccountID, conversation.ConversationID)
			}
		},
	}

	result, err := extmsg.HandleOutbound(ctx, deps, caller, extmsg.OutboundRequest{
		SessionID:        input.Body.SessionID,
		Conversation:     input.Body.Conversation,
		Text:             input.Body.Text,
		ReplyToMessageID: input.Body.ReplyToMessageID,
		IdempotencyKey:   input.Body.IdempotencyKey,
	})
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	if result != nil && result.Receipt.Delivered {
		notifyConversation := input.Body.Conversation
		if result.Receipt.Conversation != (extmsg.ConversationRef{}) {
			notifyConversation = result.Receipt.Conversation
		}
		sourceDisplay := s.extmsgSessionHandleForSelector(input.Body.SessionID)
		go s.extmsgNotifyMembers(s.backgroundCtx(), notifyConversation, sourceDisplay, "agent", input.Body.Text, input.Body.SessionID, "")
	}
	out := &ExtMsgOutboundOutput{}
	if result != nil {
		out.Body = *result
	}
	return out, nil
}

// --- Bindings ---

// humaHandleExtMsgBindingList is the Huma-typed handler for GET /v0/extmsg/bindings.
func (s *Server) humaHandleExtMsgBindingList(ctx context.Context, input *ExtMsgBindingListInput) (*ListOutput[extmsg.SessionBindingRecord], error) {
	svc, err := s.humaExtmsgServices()
	if err != nil {
		return nil, err
	}

	if input.SessionID == "" {
		return nil, huma.Error400BadRequest("session_id query parameter is required")
	}

	bindings, err := svc.Bindings.ListBySession(ctx, input.SessionID)
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	if bindings == nil {
		bindings = []extmsg.SessionBindingRecord{}
	}
	return &ListOutput[extmsg.SessionBindingRecord]{
		Index: s.latestIndex(),
		Body:  ListBody[extmsg.SessionBindingRecord]{Items: bindings, Total: len(bindings)},
	}, nil
}

// humaHandleExtMsgBind is the Huma-typed handler for POST /v0/extmsg/bind.
func (s *Server) humaHandleExtMsgBind(ctx context.Context, input *ExtMsgBindInput) (*ExtMsgBindOutput, error) {
	svc, err := s.humaExtmsgServices()
	if err != nil {
		return nil, err
	}

	caller := extmsg.Caller{Kind: extmsg.CallerController, ID: "api"}
	binding, err := svc.Bindings.Bind(ctx, caller, extmsg.BindInput{
		Conversation: input.Body.Conversation,
		SessionID:    input.Body.SessionID,
		Metadata:     input.Body.Metadata,
		Now:          time.Now(),
	})
	if err != nil {
		switch {
		case errors.Is(err, extmsg.ErrBindingConflict):
			return nil, huma.Error409Conflict(err.Error())
		case errors.Is(err, extmsg.ErrInvalidInput) || errors.Is(err, extmsg.ErrInvalidConversation):
			return nil, huma.Error400BadRequest(err.Error())
		default:
			return nil, huma.Error500InternalServerError(err.Error())
		}
	}

	s.extmsgEmitEvent()(events.ExtMsgBound, input.Body.SessionID, extmsg.BoundEventPayload{
		Provider:       input.Body.Conversation.Provider,
		ConversationID: input.Body.Conversation.ConversationID,
		SessionID:      input.Body.SessionID,
	})
	out := &ExtMsgBindOutput{}
	out.Body = binding
	return out, nil
}

// humaHandleExtMsgUnbind is the Huma-typed handler for POST /v0/extmsg/unbind.
func (s *Server) humaHandleExtMsgUnbind(ctx context.Context, input *ExtMsgUnbindInput) (*ExtMsgUnbindOutput, error) {
	svc, err := s.humaExtmsgServices()
	if err != nil {
		return nil, err
	}

	caller := extmsg.Caller{Kind: extmsg.CallerController, ID: "api"}
	unbound, err := svc.Bindings.Unbind(ctx, caller, extmsg.UnbindInput{
		Conversation: input.Body.Conversation,
		SessionID:    input.Body.SessionID,
		Now:          time.Now(),
	})
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}

	s.extmsgEmitEvent()(events.ExtMsgUnbound, input.Body.SessionID, extmsg.UnboundEventPayload{
		SessionID: input.Body.SessionID,
		Count:     len(unbound),
	})
	out := &ExtMsgUnbindOutput{}
	out.Body = ExtMsgUnbindBody{Unbound: unbound}
	return out, nil
}

// --- Groups ---

// humaHandleExtMsgGroupLookup is the Huma-typed handler for GET /v0/extmsg/groups.
func (s *Server) humaHandleExtMsgGroupLookup(ctx context.Context, input *ExtMsgGroupLookupInput) (*ExtMsgGroupOutput, error) {
	svc, err := s.humaExtmsgServices()
	if err != nil {
		return nil, err
	}

	ref := extmsg.ConversationRef{
		ScopeID:        input.ScopeID,
		Provider:       input.Provider,
		AccountID:      input.AccountID,
		ConversationID: input.ConversationID,
		Kind:           extmsg.ConversationKind(input.Kind),
	}

	caller := extmsg.Caller{Kind: extmsg.CallerController, ID: "api"}
	group, err := svc.Groups.FindByConversation(ctx, caller, ref)
	if err != nil {
		if errors.Is(err, extmsg.ErrGroupNotFound) {
			return nil, huma.Error404NotFound("group not found for conversation")
		}
		return nil, huma.Error500InternalServerError(err.Error())
	}
	out := &ExtMsgGroupOutput{}
	if group != nil {
		out.Body = *group
	}
	return out, nil
}

// humaHandleExtMsgGroupEnsure is the Huma-typed handler for POST /v0/extmsg/groups.
func (s *Server) humaHandleExtMsgGroupEnsure(ctx context.Context, input *ExtMsgGroupEnsureInput) (*ExtMsgGroupEnsureOutput, error) {
	svc, err := s.humaExtmsgServices()
	if err != nil {
		return nil, err
	}

	mode := input.Body.Mode
	if mode == "" {
		mode = extmsg.GroupModeLauncher
	}

	caller := extmsg.Caller{Kind: extmsg.CallerController, ID: "api"}
	group, err := svc.Groups.EnsureGroup(ctx, caller, extmsg.EnsureGroupInput{
		RootConversation: input.Body.RootConversation,
		Mode:             mode,
		DefaultHandle:    input.Body.DefaultHandle,
		Metadata:         input.Body.Metadata,
	})
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}

	s.extmsgEmitEvent()(events.ExtMsgGroupCreated, group.ID, extmsg.GroupCreatedEventPayload{
		Provider:       input.Body.RootConversation.Provider,
		ConversationID: input.Body.RootConversation.ConversationID,
		Mode:           string(mode),
	})
	out := &ExtMsgGroupEnsureOutput{}
	out.Body = group
	return out, nil
}

// --- Participants ---

// humaHandleExtMsgParticipantUpsert is the Huma-typed handler for POST /v0/extmsg/participants.
func (s *Server) humaHandleExtMsgParticipantUpsert(ctx context.Context, input *ExtMsgParticipantUpsertInput) (*ExtMsgParticipantOutput, error) {
	svc, err := s.humaExtmsgServices()
	if err != nil {
		return nil, err
	}

	caller := extmsg.Caller{Kind: extmsg.CallerController, ID: "api"}
	participant, err := svc.Groups.UpsertParticipant(ctx, caller, extmsg.UpsertParticipantInput{
		GroupID:   input.Body.GroupID,
		Handle:    input.Body.Handle,
		SessionID: input.Body.SessionID,
		Public:    input.Body.Public,
		Metadata:  input.Body.Metadata,
	})
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	out := &ExtMsgParticipantOutput{}
	out.Body = participant
	return out, nil
}

// humaHandleExtMsgParticipantRemove is the Huma-typed handler for DELETE /v0/extmsg/participants.
func (s *Server) humaHandleExtMsgParticipantRemove(ctx context.Context, input *ExtMsgParticipantRemoveInput) (*OKResponse, error) {
	svc, err := s.humaExtmsgServices()
	if err != nil {
		return nil, err
	}

	caller := extmsg.Caller{Kind: extmsg.CallerController, ID: "api"}
	err = svc.Groups.RemoveParticipant(ctx, caller, extmsg.RemoveParticipantInput{
		GroupID: input.Body.GroupID,
		Handle:  input.Body.Handle,
	})
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	out := &OKResponse{}
	out.Body.Status = "removed"
	return out, nil
}

// --- Transcript ---

// humaHandleExtMsgTranscriptList is the Huma-typed handler for GET /v0/extmsg/transcript.
func (s *Server) humaHandleExtMsgTranscriptList(ctx context.Context, input *ExtMsgTranscriptListInput) (*ListOutput[extmsg.ConversationTranscriptRecord], error) {
	svc, err := s.humaExtmsgServices()
	if err != nil {
		return nil, err
	}

	ref := extmsg.ConversationRef{
		ScopeID:              input.ScopeID,
		Provider:             input.Provider,
		AccountID:            input.AccountID,
		ConversationID:       input.ConversationID,
		ParentConversationID: input.ParentConversationID,
		Kind:                 extmsg.ConversationKind(input.Kind),
	}

	caller := extmsg.Caller{Kind: extmsg.CallerController, ID: "api"}
	entries, err := svc.Transcript.List(ctx, extmsg.ListTranscriptInput{
		Caller:        caller,
		Conversation:  ref,
		AfterSequence: input.AfterSequence,
		Limit:         input.Limit,
		Order:         extmsg.TranscriptOrder(input.Order),
	})
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	if entries == nil {
		entries = []extmsg.ConversationTranscriptRecord{}
	}
	return &ListOutput[extmsg.ConversationTranscriptRecord]{
		Index: s.latestIndex(),
		Body:  ListBody[extmsg.ConversationTranscriptRecord]{Items: entries, Total: len(entries)},
	}, nil
}

// humaHandleExtMsgTranscriptAck is the Huma-typed handler for POST /v0/extmsg/transcript/ack.
func (s *Server) humaHandleExtMsgTranscriptAck(ctx context.Context, input *ExtMsgTranscriptAckInput) (*OKResponse, error) {
	svc, err := s.humaExtmsgServices()
	if err != nil {
		return nil, err
	}

	caller := extmsg.Caller{Kind: extmsg.CallerController, ID: "api"}
	err = svc.Transcript.Ack(ctx, extmsg.AckMembershipInput{
		Caller:       caller,
		Conversation: input.Body.Conversation,
		SessionID:    input.Body.SessionID,
		Sequence:     input.Body.Sequence,
	})
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	out := &OKResponse{}
	out.Body.Status = "acked"
	return out, nil
}

// --- Adapters ---

// extmsgAdapterInfo is the response shape for each entry in GET /v0/extmsg/adapters.
type extmsgAdapterInfo struct {
	Provider  string `json:"provider" doc:"Adapter provider key."`
	AccountID string `json:"account_id" doc:"Adapter account ID."`
	Name      string `json:"name" doc:"Adapter display name."`
}

// humaHandleExtMsgAdapterList is the Huma-typed handler for GET /v0/extmsg/adapters.
func (s *Server) humaHandleExtMsgAdapterList(_ context.Context, _ *ExtMsgAdapterListInput) (*ListOutput[extmsgAdapterInfo], error) {
	reg, err := s.humaExtmsgAdapterRegistry()
	if err != nil {
		return nil, err
	}

	keys := reg.List()
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Provider != keys[j].Provider {
			return keys[i].Provider < keys[j].Provider
		}
		return keys[i].AccountID < keys[j].AccountID
	})
	items := make([]extmsgAdapterInfo, 0, len(keys))
	for _, k := range keys {
		a := reg.Lookup(k)
		name := ""
		if a != nil {
			name = a.Name()
		}
		items = append(items, extmsgAdapterInfo{
			Provider:  k.Provider,
			AccountID: k.AccountID,
			Name:      name,
		})
	}
	return &ListOutput[extmsgAdapterInfo]{
		Index: s.latestIndex(),
		Body:  ListBody[extmsgAdapterInfo]{Items: items, Total: len(items)},
	}, nil
}

// humaHandleExtMsgAdapterRegister is the Huma-typed handler for POST /v0/extmsg/adapters.
func (s *Server) humaHandleExtMsgAdapterRegister(_ context.Context, input *ExtMsgAdapterRegisterInput) (*ExtMsgAdapterRegisterOutput, error) {
	reg, err := s.humaExtmsgAdapterRegistry()
	if err != nil {
		return nil, err
	}

	name := input.Body.Name
	if name == "" {
		name = input.Body.Provider + "/" + input.Body.AccountID
	}

	adapter := extmsg.NewHTTPAdapter(name, input.Body.CallbackURL, input.Body.Capabilities)
	key := extmsg.AdapterKey{Provider: input.Body.Provider, AccountID: input.Body.AccountID}
	reg.Register(key, adapter)

	s.extmsgEmitEvent()(events.ExtMsgAdapterAdded, name, extmsg.AdapterEventPayload{
		Provider:  input.Body.Provider,
		AccountID: input.Body.AccountID,
	})
	out := &ExtMsgAdapterRegisterOutput{}
	out.Body.Status = "registered"
	out.Body.Provider = input.Body.Provider
	out.Body.AccountID = input.Body.AccountID
	out.Body.Name = name
	return out, nil
}

// humaHandleExtMsgAdapterUnregister is the Huma-typed handler for DELETE /v0/extmsg/adapters.
func (s *Server) humaHandleExtMsgAdapterUnregister(_ context.Context, input *ExtMsgAdapterUnregisterInput) (*OKResponse, error) {
	reg, err := s.humaExtmsgAdapterRegistry()
	if err != nil {
		return nil, err
	}

	key := extmsg.AdapterKey{Provider: input.Body.Provider, AccountID: input.Body.AccountID}
	reg.Unregister(key)

	s.extmsgEmitEvent()(events.ExtMsgAdapterRemoved, input.Body.Provider+"/"+input.Body.AccountID, extmsg.AdapterEventPayload{
		Provider:  input.Body.Provider,
		AccountID: input.Body.AccountID,
	})
	out := &OKResponse{}
	out.Body.Status = "unregistered"
	return out, nil
}

// --- Supervisor-level connected-client handlers ---

// extmsgCityState returns the State of the first running city that has extmsg
// services enabled. Returns nil when no such city is available.
func (sm *SupervisorMux) extmsgCityState() State {
	for _, c := range sm.resolver.ListCities() {
		if !c.Running {
			continue
		}
		st := sm.resolver.CityState(c.Name)
		if st != nil && st.ExtMsgServices() != nil {
			return st
		}
	}
	return nil
}

// humaHandleExtMsgClientRegister is the Huma-typed handler for POST /v0/extmsg/clients.
// It registers a connected client by credential and ensures the corresponding
// adapter is registered in the city's adapter registry.
func (sm *SupervisorMux) humaHandleExtMsgClientRegister(_ context.Context, input *ExtMsgClientRegisterInput) (*ExtMsgClientRegisterOutput, error) {
	st := sm.extmsgCityState()
	if st == nil {
		return nil, huma.Error503ServiceUnavailable("external messaging not enabled")
	}
	svc := st.ExtMsgServices()
	if svc.ConnectedClients == nil {
		return nil, huma.Error503ServiceUnavailable("connected client service not available")
	}
	reg := st.AdapterRegistry()
	if reg == nil {
		return nil, huma.Error503ServiceUnavailable("adapter registry not available")
	}

	rec, created, err := svc.ConnectedClients.Register(input.Body.Credential)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	// Register adapter for this client so HandleOutbound can deliver to it.
	key := extmsg.AdapterKey{Provider: extmsg.ConnectedClientProvider, AccountID: rec.ClientID}
	if reg.Lookup(key) == nil {
		adapter := extmsg.NewConnectedClientAdapter(rec.ClientID, svc.ConnectedClients)
		reg.Register(key, adapter)
	}

	out := &ExtMsgClientRegisterOutput{}
	out.Body.ClientID = rec.ClientID
	out.Body.Token = rec.Token
	out.Body.Created = created
	return out, nil
}

// humaHandleExtMsgGlobalInbound is the Huma-typed handler for POST /v0/extmsg/inbound
// (supervisor-level). Connected clients post flat inbound messages here without a city prefix.
func (sm *SupervisorMux) humaHandleExtMsgGlobalInbound(ctx context.Context, input *ExtMsgGlobalInboundInput) (*ExtMsgInboundOutput, error) {
	st := sm.extmsgCityState()
	if st == nil {
		return nil, huma.Error503ServiceUnavailable("external messaging not enabled")
	}
	svc := st.ExtMsgServices()
	reg := st.AdapterRegistry()
	if reg == nil {
		return nil, huma.Error503ServiceUnavailable("adapter registry not available")
	}

	msg := extmsg.ExternalInboundMessage{
		Conversation: extmsg.ConversationRef{
			Provider:       input.Body.Provider,
			AccountID:      input.Body.AccountID,
			ConversationID: input.Body.ConversationID,
			Kind:           extmsg.ConversationKind(input.Body.Kind),
		},
		Actor:      input.Body.Actor,
		Text:       input.Body.Text,
		ReceivedAt: time.Now(),
	}

	deps := extmsg.InboundDeps{
		Services:  *svc,
		Registry:  reg,
		EmitEvent: nil,
	}

	result, handleErr := extmsg.HandleInboundNormalized(ctx, deps, msg)
	if handleErr != nil {
		return nil, huma.Error422UnprocessableEntity(handleErr.Error())
	}
	out := &ExtMsgInboundOutput{}
	if result != nil {
		out.Body = *result
	}
	return out, nil
}

// precheckConnectedClientSubscribe validates the token and conversation before
// the SSE stream is committed to the wire.
func (sm *SupervisorMux) precheckConnectedClientSubscribe(_ context.Context, input *ExtMsgClientSubscribeInput) error {
	st := sm.extmsgCityState()
	if st == nil {
		return huma.Error503ServiceUnavailable("external messaging not enabled")
	}
	svc := st.ExtMsgServices()
	if svc == nil || svc.ConnectedClients == nil {
		return huma.Error503ServiceUnavailable("connected client service not available")
	}
	if input.GCClientToken == "" {
		return huma.Error401Unauthorized("X-GC-Client-Token header required")
	}
	clientID := svc.ConnectedClients.LookupByToken(input.GCClientToken)
	if clientID == "" {
		return huma.Error401Unauthorized("invalid or unknown client token")
	}
	if clientID != input.AccountID {
		return huma.Error403Forbidden("token does not match account_id")
	}
	return nil
}

// streamConnectedClientSubscribe is the SSE stream handler for
// GET /v0/extmsg/clients/{account_id}/conversations/{conversation_id}/subscribe.
// It replays missed entries from transcript when Last-Event-ID is set, then
// blocks waiting for live message notifications from the adapter.
//
// Frames are written manually using FormatConnectedClientSSEMessage so that the
// wire includes an explicit "event: message" line — the Huma SSE framework omits
// it per W3C convention (message is the default), but clients that parse event:
// lines directly need it present.
func (sm *SupervisorMux) streamConnectedClientSubscribe(hctx huma.Context, input *ExtMsgClientSubscribeInput, _ sse.Sender) {
	st := sm.extmsgCityState()
	if st == nil {
		return
	}
	svc := st.ExtMsgServices()
	if svc == nil || svc.ConnectedClients == nil {
		return
	}

	w := hctx.BodyWriter()
	flusher := findFlusher(w)

	writeMsg := func(entry extmsg.ConversationTranscriptRecord, ref extmsg.ConversationRef) bool {
		event := extmsg.SSEMessageEvent{
			Version:      "1",
			Event:        "message",
			Text:         entry.Text,
			SessionID:    entry.SourceSessionID,
			Conversation: ref,
			Sequence:     entry.Sequence,
			CreatedAt:    entry.CreatedAt,
		}
		data, err := extmsg.FormatConnectedClientSSEMessage(event)
		if err != nil {
			return true // non-fatal; skip this frame
		}
		if _, err := w.Write(data); err != nil {
			return false
		}
		if flusher != nil {
			flusher.Flush()
		}
		return true
	}

	// Register subscriber before replay to avoid a race where a message arrives
	// between replay completion and subscriber registration.
	ch := svc.ConnectedClients.Subscribe(input.AccountID, input.ConversationID)
	defer svc.ConnectedClients.Unsubscribe(input.AccountID, input.ConversationID)

	// Flush headers immediately so the client knows the stream is open.
	flushSSEHeaders(hctx)

	ref := extmsg.ConversationRef{
		Provider:       extmsg.ConnectedClientProvider,
		AccountID:      input.AccountID,
		ConversationID: input.ConversationID,
		Kind:           extmsg.ConversationDM,
	}
	caller := extmsg.Caller{Kind: extmsg.CallerController, ID: "api-subscribe"}

	// Replay transcript entries after Last-Event-ID if provided.
	// Only outbound entries (replies from the session) are sent to the subscriber.
	var lastSeq int64
	afterSeq, hasLastEventID := extmsg.ParseConnectedClientLastEventID(input.LastEventID)
	if hasLastEventID {
		entries, err := svc.Transcript.List(hctx.Context(), extmsg.ListTranscriptInput{
			Caller:        caller,
			Conversation:  ref,
			AfterSequence: afterSeq,
			Limit:         500,
			Order:         extmsg.TranscriptOrderAsc,
		})
		if err == nil {
			for _, entry := range entries {
				if entry.Sequence > lastSeq {
					lastSeq = entry.Sequence
				}
				if entry.Kind != extmsg.TranscriptMessageOutbound {
					continue
				}
				if !writeMsg(entry, ref) {
					return
				}
			}
		}
	}

	// Stream live messages: wait for notifications then read new transcript entries.
	// Notifications arrive via OutboundDeps.PostTranscript, which fires after the
	// outbound entry is committed — so entries are always present when we read.
	ctx := hctx.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			entries, err := svc.Transcript.List(ctx, extmsg.ListTranscriptInput{
				Caller:        caller,
				Conversation:  ref,
				AfterSequence: lastSeq,
				Limit:         100,
				Order:         extmsg.TranscriptOrderAsc,
			})
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if entry.Sequence > lastSeq {
					lastSeq = entry.Sequence
				}
				if entry.Kind != extmsg.TranscriptMessageOutbound {
					continue
				}
				if !writeMsg(entry, ref) {
					return
				}
			}
		}
	}
}
