package usecase

import (
	"context"
	"strings"
	"time"
)

type replyObserverKey struct{}

// WithReplyObserver receives bounded cumulative previews of assistant text.
// The terminal ChatOutput remains authoritative, including after dropped frames.
func WithReplyObserver(ctx context.Context, emit func(string)) context.Context {
	return context.WithValue(ctx, replyObserverKey{}, emit)
}

func replyObserverFrom(ctx context.Context) func(string) {
	emit, ok := ctx.Value(replyObserverKey{}).(func(string))
	if !ok {
		return nil
	}
	return emit
}

// observeReplyText enables streaming for text-only sessions and tees voice
// increments without changing the text delivered to synthesis.
func observeReplyText(ctx context.Context, next func(string) error) func(string) error {
	emit := replyObserverFrom(ctx)
	if emit == nil {
		return next
	}
	var preview strings.Builder
	var last time.Time
	return func(delta string) error {
		if preview.Len() < 16384 {
			preview.WriteString(boundedDetail(delta))
			now := time.Now()
			if last.IsZero() || now.Sub(last) >= 50*time.Millisecond {
				emit(boundedDetail(preview.String()))
				last = now
			}
		}
		if next != nil {
			return next(delta)
		}
		return nil
	}
}
