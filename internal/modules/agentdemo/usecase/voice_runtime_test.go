package usecase

import (
	"context"
	"testing"

	"github.com/NSObjects/go-speech/audio"
)

func TestVoiceRuntimeSnapshotsRemainIndependent(t *testing.T) {
	encoding := audio.EncodingOpus
	calls := 0
	base := NewVoiceUsecase(nil, nil, nil, audio.Format{}).WithRuntimeResolver(
		func(context.Context) (VoiceRuntime, error) {
			calls++
			return VoiceRuntime{Format: audio.Format{Encoding: encoding, Channels: 1}}, nil
		},
	)
	first, releaseFirst, err := base.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()
	encoding = audio.EncodingPCM16LE
	second, releaseSecond, err := base.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSecond()
	again, releaseAgain, err := first.Snapshot(context.Background())
	if err != nil || again != first || calls != 2 {
		t.Fatal("a pinned request reloaded configuration")
	}
	defer releaseAgain()
	if first.asrFormat.Encoding != audio.EncodingOpus || second.asrFormat.Encoding != audio.EncodingPCM16LE {
		t.Fatal("provider update changed an in-flight snapshot")
	}
	if first.previewSlots != second.previewSlots || base.previewSlots != first.previewSlots {
		t.Fatal("preview concurrency limit must stay shared")
	}
}
