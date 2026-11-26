package usecase

import (
	"context"
	"sync"
	"sync/atomic"
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
	send    func(string) error
	sent    bool
	replied bool
	closed  bool
	err     error
}

func beginToolFeedback(ctx context.Context, cfg ToolFeedback) func() {
	state, ok := ctx.Value(toolFeedbackKey{}).(*voiceToolFeedback)
	if !ok || state == nil || cfg.WaitingMessage == "" || cfg.WaitingDelay <= 0 {
		return func() {}
	}
	var finished atomic.Bool
	timer := time.AfterFunc(cfg.WaitingDelay, func() {
		state.mu.Lock()
		defer state.mu.Unlock()
		if finished.Load() || ctx.Err() != nil || state.closed || state.sent || state.replied || state.err != nil {
			return
		}
		// Once committed to TTS, let the short phrase finish naturally.
		state.sent = true
		state.err = state.send(cfg.WaitingMessage)
	})
	return func() {
		finished.Store(true)
		timer.Stop()
	}
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
	}
	return send()
}

// finish excludes late timers before the single TTS stream is closed.
func (s *voiceToolFeedback) finish() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return s.err
}
