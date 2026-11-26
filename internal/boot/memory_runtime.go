package boot

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/samber/do/v2"

	agentdemousecase "github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
)

func (a *App) runWithMemoryScheduler(ctx context.Context) error {
	if ctx == nil || a.injector == nil {
		return a.server.Run(ctx)
	}
	scheduler, err := do.Invoke[*agentdemousecase.MemoryScheduler](a.injector)
	if optionalServiceMissing(err) {
		return a.server.Run(ctx)
	}
	if err != nil {
		return fmt.Errorf("resolve memory scheduler: %w", err)
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := scheduler.Run(workerCtx); err != nil && workerCtx.Err() == nil {
			log.Warn().Err(err).Msg("memory scheduler stopped")
		}
	}()
	defer func() {
		cancel()
		<-done
	}()
	return a.server.Run(ctx)
}
