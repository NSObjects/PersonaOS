// Package volcclone provisions Seed-TTS 3.0 cloned voice resources.
package volcclone

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/NSObjects/PersonaOS/internal/modules/catalog/domain"
)

const (
	baseURL       = "https://voice.cn-beijing.volces.com"
	clonePath     = "/api/v3/tts/voice_clone"
	queryPath     = "/api/v3/tts/get_voice"
	customSpeaker = "custom_speaker_id"
)

// Client calls the official provisioning API, never a browser-supplied URL.
type Client struct {
	key  string
	base string
	http *http.Client
}

// New constructs the provisioning client with server-only credentials.
func New(key string) *Client {
	return &Client{key: key, base: baseURL, http: &http.Client{
		Timeout:       90 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

type requestBody struct {
	SpeakerID       string       `json:"speaker_id"`
	CustomSpeakerID string       `json:"custom_speaker_id,omitempty"`
	Audio           *sample      `json:"audio,omitempty"`
	Language        *int         `json:"language,omitempty"`
	Extra           *extraParams `json:"extra_params,omitempty"`
}

type sample struct {
	Data   []byte `json:"data"`
	Format string `json:"format"`
}

type extraParams struct {
	PromptText string `json:"prompt_text"`
}

type responseBody struct {
	Code          int    `json:"code"`
	SpeakerID     string `json:"speaker_id"`
	Status        int    `json:"status"`
	TrainingTimes int    `json:"available_training_times"`
	Speakers      []struct {
		ModelType int    `json:"model_type"`
		DemoAudio string `json:"demo_audio"`
	} `json:"speaker_status"`
}

// Train uploads one authorized sample to a new custom postpaid speaker.
func (c *Client) Train(ctx context.Context, speakerID, format, transcript string, data []byte) (domain.VoiceCloneStatus, error) {
	language := 0 // This Demo initially supports Mandarin training.
	body := requestBody{
		SpeakerID: customSpeaker, CustomSpeakerID: speakerID,
		Audio: &sample{Data: data, Format: format}, Language: &language,
	}
	if transcript != "" {
		body.Extra = &extraParams{PromptText: transcript}
	}
	return c.call(ctx, clonePath, body)
}

// Query checks the same custom speaker without starting training or synthesis.
func (c *Client) Query(ctx context.Context, speakerID string) (domain.VoiceCloneStatus, error) {
	return c.call(ctx, queryPath, requestBody{SpeakerID: customSpeaker, CustomSpeakerID: speakerID})
}

func (c *Client) call(ctx context.Context, path string, body requestBody) (domain.VoiceCloneStatus, error) {
	var status domain.VoiceCloneStatus
	payload, err := json.Marshal(body)
	if err != nil {
		return status, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(payload))
	if err != nil {
		return status, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", c.key)
	req.Header.Set("X-Api-Request-Id", uuid.NewString())
	res, err := c.http.Do(req)
	if err != nil {
		return status, fmt.Errorf("voice clone request: %w", err)
	}
	defer func() {
		if closeErr := res.Body.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("close voice clone response")
		}
	}()
	raw, err := io.ReadAll(io.LimitReader(res.Body, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 {
		return status, fmt.Errorf("invalid voice clone response size")
	}
	var out responseBody
	if err := json.Unmarshal(raw, &out); err != nil {
		if res.StatusCode != http.StatusOK {
			return status, fmt.Errorf("voice clone HTTP %d (log id: %s; non-JSON error response)", res.StatusCode, res.Header.Get("X-Tt-Logid"))
		}
		return status, fmt.Errorf("decode voice clone response: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		// Only the documented not-found code on a query may represent a
		// new voice. Never treat a remote 5xx as permission to train.
		if path == queryPath && res.StatusCode >= 400 && res.StatusCode < 500 && out.Code == 45001107 {
			return status, nil
		}
		return status, fmt.Errorf("voice clone HTTP %d code=%d message=%q (log id: %s)", res.StatusCode, out.Code, safeProviderMessage(out.Code), res.Header.Get("X-Tt-Logid"))
	}
	if out.Status != 0 && out.SpeakerID == "" {
		return status, fmt.Errorf("voice clone response missing speaker ID")
	}
	status = domain.VoiceCloneStatus{SpeakerID: out.SpeakerID, State: out.Status, TrainingTimes: out.TrainingTimes}
	for _, model := range out.Speakers {
		if model.ModelType == 5 {
			status.ModelReady = out.Status == 2 || out.Status == 4
			status.DemoAudio = safeDemoURL(model.DemoAudio)
			break
		}
	}
	return status, nil
}

func safeDemoURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host != "bytespeech.com" && !strings.HasSuffix(host, ".bytespeech.com") {
		return ""
	}
	return raw
}

// safeProviderMessage uses documented descriptions rather than logging arbitrary
// provider text, which can include user content or credentials.
func safeProviderMessage(code int) string {
	switch code {
	case 45001001:
		return "invalid request parameters"
	case 45001101:
		return "audio upload failed"
	case 45001102:
		return "ASR transcription failed"
	case 45001104:
		return "voiceprint check failed"
	case 45001105:
		return "audio data retrieval failed"
	case 45001107:
		return "speaker ID not found"
	case 45001108:
		return "audio transcoding failed"
	case 45001109:
		return "transcript verification failed"
	case 45001110:
		return "voice deletion failed"
	case 45001112:
		return "SNR check failed"
	case 45001113:
		return "denoise failed"
	case 45001114:
		return "poor audio quality"
	case 45001122:
		return "no speech detected"
	case 45001123:
		return "training limit reached"
	case 45001124, 45001125, 45001127, 45001128:
		return "provider content review rejected the request"
	case 45001126:
		return "invalid demo text length"
	case 55001301:
		return "provider database query failed"
	case 55001302:
		return "provider database insert failed"
	case 55001303:
		return "provider database update failed"
	case 55001304:
		return "provider database delete failed"
	case 55001305:
		return "provider object storage upload failed"
	case 55001306:
		return "provider object storage download failed"
	case 55001307:
		return "provider voice clone failed"
	default:
		return "unclassified provider error"
	}
}
