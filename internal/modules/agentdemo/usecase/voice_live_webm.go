package usecase

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/rs/zerolog/log"

	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

// RecognizeLiveCapture accepts a native Opus container. WebM is incrementally
// remuxed into short Ogg pages without decoding or re-encoding Opus.
func (u *VoiceUsecase) RecognizeLiveCapture(ctx context.Context, input io.ReadCloser, format string) (string, error) {
	switch format {
	case "ogg":
		return u.RecognizeLiveOpus(ctx, input)
	case "webm":
		return u.recognizeLiveWebM(ctx, input)
	default:
		return "", apperr.NewBadRequest("unsupported live Opus container")
	}
}

func (u *VoiceUsecase) recognizeLiveWebM(ctx context.Context, input io.ReadCloser) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, voiceTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin",
		"-probesize", "4096", "-analyzeduration", "0", "-f", "webm", "-i", "pipe:0",
		"-map", "0:a:0", "-vn", "-c:a", "copy", "-page_duration", "20000", "-flush_packets", "1", "-f", "ogg", "pipe:1")
	cmd.Stdin = input
	output, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("open live WebM remux output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start live WebM remux: %w", err)
	}
	// If the process or ASR fails, close the input to unblock os/exec's
	// stdin copy goroutine before waiting for process cleanup.
	stop := context.AfterFunc(runCtx, func() {
		if closeErr := input.Close(); closeErr != nil {
			log.Debug().Err(closeErr).Msg("interrupt live WebM input")
		}
	})
	defer stop()
	text, err := u.RecognizeLiveOpus(runCtx, output)
	if err != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	if err != nil {
		return "", err
	}
	if waitErr != nil {
		return "", fmt.Errorf("live WebM remux: %w", waitErr)
	}
	return text, nil
}
