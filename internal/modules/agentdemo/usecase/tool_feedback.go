package usecase

import (
	"context"
	"sync"
	"time"
)

// ToolFeedback is presentation metadata, not part of the Eino tool schema.
// A tool opts in after validating its arguments, just before actual work.
type ToolFeedback struct {
	WaitingMessage string
	WaitingDelay   time.Duration
}

const defaultToolWaitingDelay = 300 * time.Millisecond

type toolFeedbackKey struct{}

// voiceToolFeedback is scoped to a single voice turn. All text submissions
// share this lock; the timer never writes concurrently with the model.
type voiceToolFeedback struct {
	mu      sync.Mutex
	ctx     context.Context
	timer   *time.Timer
	send    func(string) error
	sent    bool
	replied bool
	closed  bool
	err     error
}

// beginToolFeedback arms one timer for the entire tool-to-reply interval.
// Tool completion does not stop it: the next model response may still be slow.
func beginToolFeedback(ctx context.Context, cfg ToolFeedback) {
	state, ok := ctx.Value(toolFeedbackKey{}).(*voiceToolFeedback)
	if !ok || state == nil || cfg.WaitingMessage == "" || cfg.WaitingDelay <= 0 {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.timer != nil || state.closed || state.replied || state.err != nil || state.ctx.Err() != nil {
		return
	}
	state.timer = time.AfterFunc(cfg.WaitingDelay, func() {
		state.mu.Lock()
		defer state.mu.Unlock()
		if state.ctx.Err() != nil || state.closed || state.sent || state.replied || state.err != nil {
			return
		}
		// Once committed to TTS, let the short phrase finish naturally.
		state.sent = true
		state.err = state.send(cfg.WaitingMessage)
	})
}

// reply serializes the normal text with the optional waiting phrase.
func (s *voiceToolFeedback) reply(ctx context.Context, text string, send func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.err != nil {
		return s.err
	}
	if text != "" {
		s.replied = true
		if s.timer != nil {
			s.timer.Stop()
		}
	}
	return send()
}

// finish excludes late timers before the single TTS stream is closed.
func (s *voiceToolFeedback) finish() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
	}
	return s.err
}
