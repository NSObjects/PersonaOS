package usecase

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/NSObjects/go-speech/asr"
	"github.com/NSObjects/go-speech/audio"

	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

const maxOpusDurationSamples = 16 * 48000 // Allow one second of capture margin.

var errAudioTooLarge = errors.New("audio exceeds size limit")

// prepareASRAudio preserves the Opus codec for providers accepting Ogg Opus.
// WebM/Opus needs a container-only remux; Tencent needs decoded PCM instead.
func prepareASRAudio(ctx context.Context, data []byte, mediaType string, target audio.Encoding) ([]byte, error) {
	if target != audio.EncodingOpus && target != audio.EncodingPCM16LE {
		return nil, apperr.NewBadRequest("unsupported ASR input encoding")
	}
	var result []byte
	switch mediaType {
	case "audio/ogg":
		if target == audio.EncodingOpus {
			result = data
		} else {
			decoded, err := ffmpegAudio(ctx, data, "ogg", false)
			if err != nil {
				return nil, err
			}
			result = decoded
		}
	case "audio/webm":
		converted, err := ffmpegAudio(ctx, data, "webm", target == audio.EncodingOpus)
		if err != nil {
			return nil, err
		}
		result = converted
	default:
		return nil, apperr.NewBadRequest("expected audio/ogg or audio/webm Opus")
	}
	if target == audio.EncodingOpus {
		if _, err := oggPages(result); err != nil {
			return nil, err
		}
	} else if len(result) == 0 || len(result) > maxVoicePCMBytes || len(result)%2 != 0 {
		return nil, apperr.NewBadRequest("decoded PCM recording is invalid or too long")
	}
	return result, nil
}

type sizeLimitedWriter struct {
	bytes.Buffer
	max int
}

func (w *sizeLimitedWriter) Write(data []byte) (int, error) {
	if len(data) > w.max-w.Len() {
		return 0, errAudioTooLarge
	}
	return w.Buffer.Write(data)
}

func ffmpegAudio(ctx context.Context, data []byte, source string, preserveOpus bool) ([]byte, error) {
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-f", source,
		"-i", "pipe:0", "-map", "0:a:0", "-vn",
	}
	limit := maxVoicePCMBytes
	if preserveOpus {
		args = append(args, "-c:a", "copy", "-f", "ogg", "pipe:1")
		limit = maxVoiceOpusBytes
	} else {
		args = append(args, "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", "-f", "s16le", "pipe:1")
	}
	cmd := exec.CommandContext(runCtx, "ffmpeg", args...)
	cmd.Stdin = bytes.NewReader(data)
	output := &sizeLimitedWriter{max: limit}
	cmd.Stdout = output
	// Avoid logging or exposing any captured speech or raw subprocess output.
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, apperr.New(apperr.ErrInternalServer, "ffmpeg is required for browser Opus input")
		}
		if runCtx.Err() != nil {
			return nil, runCtx.Err()
		}
		return nil, apperr.NewBadRequest("invalid or oversized Opus recording")
	}
	return output.Bytes(), nil
}

// oggPages validates a single complete Ogg Opus stream and returns whole pages.
// Bytes are not re-encoded; OpusHead is mandatory, and the decoded duration is
// bounded by the 15-second recording limit plus a one-second safety margin.
func oggPages(input []byte) ([][]byte, error) {
	pages := make([][]byte, 0, 64)
	var lastGranule uint64
	for pos := 0; pos < len(input); {
		if len(input)-pos < 27 || !bytes.Equal(input[pos:pos+4], []byte("OggS")) || input[pos+4] != 0 {
			return nil, apperr.NewBadRequest("invalid Ogg Opus page header")
		}
		segments := int(input[pos+26])
		headerLen := 27 + segments
		if headerLen > len(input)-pos {
			return nil, apperr.NewBadRequest("invalid Ogg page segments")
		}
		bodyLen := 0
		for _, n := range input[pos+27 : pos+headerLen] {
			bodyLen += int(n)
		}
		pageLen := headerLen + bodyLen
		if pageLen > len(input)-pos {
			return nil, apperr.NewBadRequest("truncated Ogg page")
		}
		if len(pages) == 0 {
			body := input[pos+headerLen : pos+pageLen]
			if !bytes.HasPrefix(body, []byte("OpusHead")) {
				return nil, apperr.NewBadRequest("expected Ogg Opus, not another codec")
			}
		}
		granule := binary.LittleEndian.Uint64(input[pos+6 : pos+14])
		if granule != ^uint64(0) {
			lastGranule = granule
		}
		pages = append(pages, input[pos:pos+pageLen])
		if len(pages) > 2000 {
			return nil, apperr.NewBadRequest("too many Ogg Opus pages")
		}
		pos += pageLen
	}
	if len(pages) < 2 || lastGranule > maxOpusDurationSamples {
		return nil, apperr.NewBadRequest("invalid or excessive Ogg Opus duration")
	}
	return pages, nil
}

// opusASRFormat uses the nominal input rate recorded in OpusHead, not
// the fixed 48 kHz Ogg granule clock. A zero input rate means unspecified.
func opusASRFormat(input []byte) (audio.Format, error) {
	pages, err := oggPages(input)
	if err != nil {
		return audio.Format{}, err
	}
	return opusPageFormat(pages[0])
}

func opusPageFormat(page []byte) (audio.Format, error) {
	header := page[27+int(page[26]):]
	if len(header) < 19 || !bytes.HasPrefix(header, []byte("OpusHead")) || header[9] != 1 {
		return audio.Format{}, apperr.NewBadRequest("expected a complete mono OpusHead")
	}
	rate := binary.LittleEndian.Uint32(header[12:16])
	if rate == 0 {
		rate = 48000
	}
	switch rate {
	case 8000, 12000, 16000, 24000, 48000:
		return audio.Format{Encoding: audio.EncodingOpus, SampleRate: int(rate), Channels: 1}, nil
	default:
		return audio.Format{}, apperr.NewBadRequest("unsupported Opus input sample rate")
	}
}

func sendOggASR(ctx context.Context, stream asr.Stream, ogg []byte) error {
	pages, err := oggPages(ogg)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for i, page := range pages {
		// Ogg identification/comment header pages are sent immediately.
		if i > 1 {
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err := stream.Send(ctx, page); err != nil {
			return fmt.Errorf("send Ogg Opus page: %w", err)
		}
	}
	return stream.CloseSend(ctx)
}

// AudioMIME reports the response's real encoding rather than a fixed MP3 MIME.
func AudioMIME(format audio.Format) string {
	switch format.Encoding {
	case audio.EncodingOpus:
		return "audio/ogg"
	case audio.EncodingMP3:
		return "audio/mpeg"
	default:
		return strings.TrimSpace(string(format.Encoding))
	}
}
