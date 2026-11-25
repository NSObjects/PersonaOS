package usecase

import (
	"context"

	"github.com/google/uuid"

	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	"github.com/NSObjects/PersonaOS/internal/platform/requestctx"
)

// Interrupt cancels the current execution, if any, and waits for its terminal
// event. The session remains bound to the same actor, role and device.
func (s *ConversationSessions) Interrupt(ctx context.Context, sessionID string) error {
	id, err := uuid.Parse(sessionID)
	if err != nil || id == uuid.Nil {
		return apperr.NewBadRequest("invalid conversation session ID")
	}
	owner, role := requestctx.GetUserID(ctx), requestctx.GetRoleID(ctx)
	if owner <= 0 || role <= 0 {
		return apperr.NewUnauthorized()
	}
	s.mu.Lock()
	session := s.sessions[id]
	s.mu.Unlock()
	if session == nil {
		return nil
	}
	if session.owner != owner || session.role != role {
		return apperr.NewNotFound("conversation session")
	}
	return session.interrupt(ctx)
}

func (s *conversationSession) interrupt(ctx context.Context) error {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return nil
	}
	s.interrupted = true
	if s.cancel != nil {
		s.cancel()
	}
	done := s.done
	s.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
