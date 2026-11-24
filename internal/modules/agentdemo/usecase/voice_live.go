package usecase

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/NSObjects/go-speech/asr"
	"github.com/NSObjects/go-speech/audio"

	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

// SupportsLiveOpus reports whether the configured recognizer accepts Ogg Opus.
func (u *VoiceUsecase) SupportsLiveOpus() bool {
	return u != nil && u.recognizer != nil && u.asrFormat.Encoding == audio.EncodingOpus
}

// RecognizeLiveOpus reads a continuous Ogg stream from an already paced source.
// The SDK receives each complete page immediately, not after the input ends.
// The transport owns the reader and must enforce an input read deadline.
func (u *VoiceUsecase) RecognizeLiveOpus(ctx context.Context, input io.Reader) (string, error) {
	if !u.SupportsLiveOpus() || ctx == nil || input == nil {
		return "", apperr.NewBadRequest("live Ogg Opus ASR is unavailable")
	}
	runCtx, cancel := context.WithTimeout(ctx, voiceTimeout)
	defer cancel()
	pages := &liveOggReader{input: input}
	first, err := pages.next()
	if err != nil {
		return "", apperr.NewBadRequest("missing or invalid OpusHead")
	}
	format, err := opusPageFormat(first)
	if err != nil {
		return "", err
	}
	if u.asrFormat.SampleRate != 0 {
		format.SampleRate = u.asrFormat.SampleRate
	}
	return u.recognizeStream(runCtx, cancel, format, func(stream asr.Stream) error {
		return sendLiveOgg(runCtx, stream, pages, first)
	})
}

type liveOggReader struct {
	input io.Reader
	total int
	count int
}

func (r *liveOggReader) next() ([]byte, error) {
	header := make([]byte, 27)
	if _, err := io.ReadFull(r.input, header); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		return nil, apperr.WrapBadRequest(err, "truncated Ogg page header")
	}
	if !bytes.HasPrefix(header, []byte("OggS")) || header[4] != 0 {
		return nil, apperr.NewBadRequest("invalid Ogg page header")
	}
	segments := make([]byte, int(header[26]))
	if _, err := io.ReadFull(r.input, segments); err != nil {
		return nil, apperr.WrapBadRequest(err, "truncated Ogg page segments")
	}
	bodyLen := 0
	for _, n := range segments {
		bodyLen += int(n)
	}
	length := len(header) + len(segments) + bodyLen
	granule := binary.LittleEndian.Uint64(header[6:14])
	if length > maxVoiceOpusBytes-r.total || r.count >= 2000 ||
		(granule != ^uint64(0) && granule > maxOpusDurationSamples) {
		return nil, apperr.NewBadRequest("excessive Ogg Opus stream")
	}
	page := make([]byte, length)
	copy(page, header)
	copy(page[len(header):], segments)
	if _, err := io.ReadFull(r.input, page[len(header)+len(segments):]); err != nil {
		return nil, apperr.WrapBadRequest(err, "truncated Ogg page body")
	}
	r.total += length
	r.count++
	return page, nil
}

func sendLiveOgg(ctx context.Context, stream asr.Stream, pages *liveOggReader, first []byte) error {
	if err := stream.Send(ctx, first); err != nil {
		return fmt.Errorf("send live OpusHead: %w", err)
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		page, err := pages.next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if err := stream.Send(ctx, page); err != nil {
			return fmt.Errorf("send live Opus page: %w", err)
		}
	}
	if pages.count < 3 {
		return apperr.NewBadRequest("missing Opus audio pages")
	}
	return stream.CloseSend(ctx)
}
