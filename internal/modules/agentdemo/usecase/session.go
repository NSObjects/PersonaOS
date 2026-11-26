package usecase

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
	"github.com/NSObjects/PersonaOS/internal/platform/requestctx"
)

const (
	maxConversationSessions = 64
	maxSessionEvents        = 256
	sessionIdleTimeout      = 15 * time.Minute
	sessionTurnStart        = "turn_start"
	sessionTurnDone         = "turn_done"
	sessionTurnError        = "turn_error"
	sessionTrace            = "trace"
	sessionReplyPreview     = "reply_preview"
)

// SessionEvent is a transport-independent event in a conversation. Sequence
// is session-local; invocation IDs in Trace are turn-local.
type SessionEvent struct {
	Sequence uint64          `json:"sequence"`
	Turn     uint64          `json:"turn"`
	Type     string          `json:"type"`
	TimeMS   int64           `json:"time_ms"`
	Message  string          `json:"message,omitempty"`
	Preview  string          `json:"preview,omitempty"`
	Trace    *ExecutionEvent `json:"trace,omitempty"`
	Reply    *ChatOutput     `json:"reply,omitempty"`
}

// SessionEvents returns an ordered bounded replay with an explicit gap flag.
type SessionEvents struct {
	Events []SessionEvent `json:"events"`
	Gap    bool           `json:"gap"`
}

// ConversationSessions owns bounded single-process Demo conversations.
// A subscriber never owns the execution context or retains an audio stream.
type ConversationSessions struct {
	mu       sync.Mutex
	sessions map[uuid.UUID]*conversationSession
}

type conversationSession struct {
	mu          sync.Mutex
	owner       int64
	role        int64
	deviceType  int64
	lastUsed    time.Time
	active      bool
	interrupted bool
	cancel      context.CancelFunc
	done        chan struct{}
	turn        uint64
	sequence    uint64
	history     []Message
	events      []SessionEvent
	changed     chan struct{}
}

// NewConversationSessions creates the shared session registry.
func NewConversationSessions() *ConversationSessions {
	return &ConversationSessions{sessions: make(map[uuid.UUID]*conversationSession)}
}

// ConversationTurn reserves one execution in a session and carries the
// authoritative history, independent of an observer's subscription.
type ConversationTurn struct {
	session *conversationSession
	input   ChatInput
	number  uint64
	started bool
}

// Begin validates the actor and binds a UUID to exactly one device type.
// A second concurrent turn is rejected rather than racing history updates.
func (s *ConversationSessions) Begin(ctx context.Context, in ChatInput) (*ConversationTurn, error) {
	in.History = nil
	if err := validateChatInput(ctx, in); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(in.SessionID)
	if err != nil || id == uuid.Nil {
		return nil, apperr.NewBadRequest("valid conversation session ID required")
	}
	in.SessionID = id.String()
	owner, role := requestctx.GetUserID(ctx), requestctx.GetRoleID(ctx)
	if owner <= 0 || role <= 0 {
		return nil, apperr.NewUnauthorized()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	session := s.sessions[id]
	if session == nil {
		if len(s.sessions) >= maxConversationSessions {
			return nil, apperr.NewConflict("conversation capacity reached")
		}
		session = &conversationSession{owner: owner, role: role, deviceType: in.DeviceTypeID, changed: make(chan struct{})}
		s.sessions[id] = session
	}
	return session.begin(owner, role, in)
}

func (s *ConversationSessions) prune() {
	now := time.Now()
	for id, session := range s.sessions {
		session.mu.Lock()
		expired := !session.active && now.Sub(session.lastUsed) > sessionIdleTimeout
		session.mu.Unlock()
		if expired {
			delete(s.sessions, id)
		}
	}
}

func (s *conversationSession) begin(owner, role int64, in ChatInput) (*ConversationTurn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owner != owner || s.role != role {
		return nil, apperr.NewNotFound("conversation session")
	}
	if s.deviceType != in.DeviceTypeID || s.active {
		return nil, apperr.NewConflict("conversation is busy or belongs to another device type")
	}
	s.active = true
	s.interrupted = false
	s.done = make(chan struct{})
	s.turn++
	in.History = slices.Clone(s.history)
	in.Message = strings.TrimSpace(in.Message)
	s.publish(SessionEvent{Turn: s.turn, Type: sessionTurnStart, Message: in.Message})
	return &ConversationTurn{session: s, input: in, number: s.turn}, nil
}

// publish requires the session lock. Closing the signal wakes all observers;
// no subscriber queue can apply backpressure to an Agent or device.
func (s *conversationSession) publish(event SessionEvent) {
	s.sequence++
	event.Sequence = s.sequence
	event.TimeMS = time.Now().UnixMilli()
	s.lastUsed = time.Now()
	if len(s.events) == maxSessionEvents {
		copy(s.events, s.events[1:])
		s.events = s.events[:maxSessionEvents-1]
	}
	s.events = append(s.events, event)
	close(s.changed)
	s.changed = make(chan struct{})
}

// Run executes the reserved turn once, forwarding native trace events to both
// the session and any request-local transport observer. Call immediately after Begin.
func (t *ConversationTurn) Run(ctx context.Context, run func(context.Context, ChatInput) (ChatOutput, error)) (ChatOutput, error) {
	t.session.mu.Lock()
	if t.started {
		t.session.mu.Unlock()
		return ChatOutput{}, apperr.NewConflict("conversation turn already started")
	}
	t.started = true
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	t.session.cancel = cancel
	if t.session.interrupted {
		cancel()
	}
	t.session.mu.Unlock()
	outer := observerFrom(ctx)
	ctx = WithExecutionObserver(ctx, func(event ExecutionEvent) {
		if t.trace(event) && outer != nil {
			outer.emit(event)
		}
	})
	outerReply := replyObserverFrom(ctx)
	ctx = WithReplyObserver(ctx, func(preview string) {
		if t.preview(preview) && outerReply != nil {
			outerReply(preview)
		}
	})
	var out ChatOutput
	err := ctx.Err()
	if err == nil {
		out, err = run(ctx, t.input)
	}
	if err == nil {
		err = ctx.Err()
	}
	return out, t.finish(out, err)
}

func (t *ConversationTurn) preview(text string) bool {
	return t.publish(SessionEvent{Turn: t.number, Type: sessionReplyPreview, Preview: text})
}

func (t *ConversationTurn) trace(event ExecutionEvent) bool {
	return t.publish(SessionEvent{Turn: t.number, Type: sessionTrace, Trace: &event})
}

func (t *ConversationTurn) publish(event SessionEvent) bool {
	s := t.session
	s.mu.Lock()
	defer s.mu.Unlock()
	// A canceled Eino stream may finish its callback after the turn ends.
	// Do not append late events behind a terminal record or into a new turn.
	if !s.active || s.turn != t.number {
		return false
	}
	s.publish(event)
	return true
}

func (t *ConversationTurn) finish(out ChatOutput, err error) error {
	s := t.session
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interrupted {
		err = context.Canceled
	}
	event := SessionEvent{Turn: t.number, Type: sessionTurnError}
	if err == nil {
		event.Type = sessionTurnDone
		event.Reply = &out
		reply := recentReplyExcerpt(t.input.Message, out.Reply)
		s.history = append(s.history, Message{Role: string(schema.User), Content: t.input.Message}, Message{Role: string(schema.Assistant), Content: reply})
		s.history = trimRecentHistory(s.history)
	}
	s.active = false
	s.cancel = nil
	s.publish(event)
	close(s.done)
	return err
}

// Read waits for events after a sequence cursor without owning the active turn.
// Only the administrator and active role that opened the conversation may read it.
func (s *ConversationSessions) Read(ctx context.Context, sessionID string, after uint64) (SessionEvents, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return SessionEvents{}, apperr.NewBadRequest("invalid conversation session ID")
	}
	s.mu.Lock()
	s.prune()
	session := s.sessions[id]
	s.mu.Unlock()
	if session == nil || session.owner != requestctx.GetUserID(ctx) || session.role != requestctx.GetRoleID(ctx) {
		return SessionEvents{}, apperr.NewNotFound("conversation session")
	}
	return session.read(ctx, after)
}

func (s *conversationSession) read(ctx context.Context, after uint64) (SessionEvents, error) {
	for {
		s.mu.Lock()
		if after > s.sequence {
			s.mu.Unlock()
			return SessionEvents{}, apperr.NewBadRequest("invalid event cursor")
		}
		batch := SessionEvents{Events: make([]SessionEvent, 0, len(s.events))}
		if len(s.events) > 0 && after < s.events[0].Sequence-1 {
			batch.Gap = true
		}
		for _, event := range s.events {
			if event.Sequence > after {
				batch.Events = append(batch.Events, event)
			}
		}
		changed := s.changed
		s.mu.Unlock()
		if len(batch.Events) > 0 {
			return batch, nil
		}
		select {
		case <-ctx.Done():
			return batch, ctx.Err()
		case <-changed:
		}
	}
}
