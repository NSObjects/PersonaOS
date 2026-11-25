package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
	"github.com/NSObjects/PersonaOS/internal/platform/apperr"
)

// MaxCloneSampleBytes is the maximum accepted reference recording size.
const MaxCloneSampleBytes = 10_000_000

// VoiceCloner provisions a voice, independent of paid text synthesis.
type VoiceCloner interface {
	Train(context.Context, string, string, string, []byte) (domain.VoiceCloneStatus, error)
	Query(context.Context, string) (domain.VoiceCloneStatus, error)
}

// WithVoiceCloner installs the server-configured cloning adapter.
func (u *ResourceUsecase) WithVoiceCloner(cloner VoiceCloner) *ResourceUsecase {
	u.cloner = cloner
	return u
}

// CreateClone reserves a durable custom speaker identity before any remote call.
// It remains disabled until the separate Seed-TTS 3.0 activation flow exists.
func (u *ResourceUsecase) CreateClone(ctx context.Context, name string) (domain.Resource, error) {
	if err := u.ready(ctx); err != nil {
		return domain.Resource{}, err
	}
	if u.cloner == nil {
		return domain.Resource{}, apperr.NewBadRequest("声音复刻服务未配置")
	}
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	value, err := domain.NewResource(0, domain.ResourceVoice, "clone-"+id, name, "",
		domain.Seed3CloneProvider, "PersonaOS_"+id, false, time.Time{}, time.Time{})
	if err != nil {
		return domain.Resource{}, apperr.WrapBadRequest(err, "invalid clone resource")
	}
	return u.store.Create(ctx, value)
}

func (u *ResourceUsecase) cloneResource(ctx context.Context, id int64) (domain.Resource, error) {
	resource, err := u.Find(ctx, id)
	if err != nil {
		return domain.Resource{}, err
	}
	if resource.Kind != domain.ResourceVoice || resource.Provider != domain.Seed3CloneProvider {
		return domain.Resource{}, apperr.NewBadRequest("not a cloned voice")
	}
	if u.cloner == nil {
		return domain.Resource{}, apperr.NewBadRequest("声音复刻服务未配置")
	}
	return resource, nil
}

// TrainClone accepts only explicitly authorized samples and never auto-retries.
// A published or activated voice must not be overwritten.
func (u *ResourceUsecase) TrainClone(ctx context.Context, id int64, format, transcript string, data []byte, authorized bool) (domain.VoiceCloneStatus, error) {
	resource, err := u.cloneResource(ctx, id)
	if err != nil {
		return domain.VoiceCloneStatus{}, err
	}
	if !authorized || len(data) == 0 || len(data) > MaxCloneSampleBytes || len(transcript) > 4000 ||
		(format != "wav" && format != "mp3") {
		return domain.VoiceCloneStatus{}, apperr.NewBadRequest("需要授权录音，WAV/MP3 不超过 10 MB")
	}
	select {
	case u.cloneSlots <- struct{}{}:
		defer func() { <-u.cloneSlots }()
	default:
		return domain.VoiceCloneStatus{}, apperr.NewConflict("已有声音复刻请求正在执行")
	}
	if resource.Enabled {
		return domain.VoiceCloneStatus{}, apperr.NewConflict("已启用音色不可重新训练")
	}
	status, err := u.cloner.Query(ctx, resource.Reference)
	if err != nil {
		return status, apperr.WrapInternal(err, "check clone before training")
	}
	if status.State != 0 && status.State != 2 {
		return status, apperr.NewConflict("音色已激活或处于未知状态")
	}
	if status.State == 2 && status.TrainingTimes <= 0 {
		return status, apperr.NewConflict("音色没有剩余训练次数")
	}
	status, err = u.cloner.Train(ctx, resource.Reference, format, strings.TrimSpace(transcript), data)
	if err != nil {
		return status, apperr.WrapInternal(err, "train clone")
	}
	return status, nil
}

// CloneStatus queries live provider status and the expiring audition URL.
func (u *ResourceUsecase) CloneStatus(ctx context.Context, id int64) (domain.VoiceCloneStatus, error) {
	resource, err := u.cloneResource(ctx, id)
	if err != nil {
		return domain.VoiceCloneStatus{}, err
	}
	status, err := u.cloner.Query(ctx, resource.Reference)
	if err != nil {
		return status, apperr.WrapInternal(err, "query clone")
	}
	return status, nil
}
