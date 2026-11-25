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

// providerMessages contains safe descriptions instead of arbitrary upstream
// text, which may contain user content or credentials.
var providerMessages = map[int]string{
	45001001: "invalid request parameters",
	45001101: "audio upload failed",
	45001102: "ASR transcription failed",
	45001104: "voiceprint check failed",
	45001105: "audio data retrieval failed",
	45001107: "speaker ID not found",
	45001108: "audio transcoding failed",
	45001109: "transcript verification failed",
	45001110: "voice deletion failed",
	45001112: "SNR check failed",
	45001113: "denoise failed",
	45001114: "poor audio quality",
	45001122: "no speech detected",
	45001123: "training limit reached",
	45001124: "provider content review rejected the request",
	45001125: "provider content review rejected the request",
	45001127: "provider content review rejected the request",
	45001128: "provider content review rejected the request",
	45001126: "invalid demo text length",
	55001301: "provider database query failed",
	55001302: "provider database insert failed",
	55001303: "provider database update failed",
	55001304: "provider database delete failed",
	55001305: "provider object storage upload failed",
	55001306: "provider object storage download failed",
	55001307: "provider voice clone failed",
}

func safeProviderMessage(code int) string {
	if message, ok := providerMessages[code]; ok {
		return message
	}
	return "unclassified provider error"
}
