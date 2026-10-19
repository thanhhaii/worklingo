// Package chat persists messages and broadcasts them over WebSocket.
package chat

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/worklingo/backend/internal/dto"
	"github.com/worklingo/backend/internal/store"
	"github.com/worklingo/backend/internal/ws"
)

const (
	AuthorUser    = "USER"
	AuthorPersona = "PERSONA"
	AuthorSystem  = "SYSTEM"
)

type Service struct {
	q   *store.Queries
	hub ws.Broadcaster
}

func NewService(q *store.Queries, hub ws.Broadcaster) *Service {
	return &Service{q: q, hub: hub}
}

func (s *Service) create(ctx context.Context, p store.CreateMessageParams) (store.Message, error) {
	return s.q.CreateMessage(ctx, p)
}

// PostUser stores a user message and broadcasts message.new.
func (s *Service) PostUser(ctx context.Context, ch store.Channel, userID uuid.UUID, content string, parentID *uuid.UUID) (store.Message, error) {
	m, err := s.create(ctx, store.CreateMessageParams{
		SessionID: ch.SessionID, ChannelID: ch.ID, ParentID: parentID,
		AuthorType: AuthorUser, UserID: &userID, Content: content,
	})
	if err != nil {
		return m, err
	}
	s.hub.Broadcast(ch.SessionID, ws.TypeMessageNew, dto.FromMessage(m))
	return m, nil
}

// PostSystem stores a SYSTEM message and broadcasts it.
func (s *Service) PostSystem(ctx context.Context, sessionID, channelID uuid.UUID, content string, parentID *uuid.UUID) (store.Message, error) {
	m, err := s.create(ctx, store.CreateMessageParams{
		SessionID: sessionID, ChannelID: channelID, ParentID: parentID,
		AuthorType: AuthorSystem, Content: content,
	})
	if err != nil {
		return m, err
	}
	s.hub.Broadcast(sessionID, ws.TypeMessageNew, dto.FromMessage(m))
	return m, nil
}

// PostPersona stores a complete (non-streamed) persona message and broadcasts it.
func (s *Service) PostPersona(ctx context.Context, sessionID, channelID uuid.UUID, personaID, content string, parentID *uuid.UUID) (store.Message, error) {
	m, err := s.create(ctx, store.CreateMessageParams{
		SessionID: sessionID, ChannelID: channelID, ParentID: parentID,
		AuthorType: AuthorPersona, PersonaID: &personaID, Content: content,
	})
	if err != nil {
		return m, err
	}
	s.hub.Broadcast(sessionID, ws.TypeMessageNew, dto.FromMessageStreaming(m, false))
	s.hub.Broadcast(sessionID, ws.TypeMessageDone, dto.FromMessageStreaming(m, false))
	return m, nil
}

// StartPersonaStream creates an empty persona message and broadcasts message.new (streaming:true).
func (s *Service) StartPersonaStream(ctx context.Context, sessionID, channelID uuid.UUID, personaID string, parentID *uuid.UUID) (store.Message, error) {
	m, err := s.create(ctx, store.CreateMessageParams{
		SessionID: sessionID, ChannelID: channelID, ParentID: parentID,
		AuthorType: AuthorPersona, PersonaID: &personaID, Content: "",
	})
	if err != nil {
		return m, err
	}
	s.hub.Broadcast(sessionID, ws.TypeMessageNew, dto.FromMessageStreaming(m, true))
	return m, nil
}

// Delta broadcasts a streamed content chunk.
func (s *Service) Delta(m store.Message, delta string) {
	s.hub.Broadcast(m.SessionID, ws.TypeMessageDelta, map[string]string{
		"messageId": m.ID.String(), "channelId": m.ChannelID.String(), "delta": delta,
	})
}

// FinishPersonaStream persists the final content and broadcasts message.done.
func (s *Service) FinishPersonaStream(ctx context.Context, m store.Message, content string) (store.Message, error) {
	out, err := s.q.UpdateMessageContent(ctx, store.UpdateMessageContentParams{ID: m.ID, Content: content})
	if err != nil {
		// still tell clients the stream is over
		m.Content = content
		s.hub.Broadcast(m.SessionID, ws.TypeMessageDone, dto.FromMessageStreaming(m, false))
		return m, err
	}
	s.hub.Broadcast(m.SessionID, ws.TypeMessageDone, dto.FromMessageStreaming(out, false))
	return out, nil
}

// Typing broadcasts a persona typing indicator.
func (s *Service) Typing(sessionID, channelID uuid.UUID, personaID string) {
	s.hub.Broadcast(sessionID, ws.TypeTyping, map[string]string{"channelId": channelID.String(), "personaId": personaID})
}

// History returns the last n messages of a channel in ascending order.
func (s *Service) History(ctx context.Context, channelID uuid.UUID, n int) ([]store.Message, error) {
	return s.q.ListMessagesBefore(ctx, store.ListMessagesBeforeParams{
		ChannelID: channelID, CreatedAt: time.Now().Add(time.Hour), Limit: int32(n),
	})
}
