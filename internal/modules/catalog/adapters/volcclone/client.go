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
	if res.StatusCode != http.StatusOK {
		return status, fmt.Errorf("voice clone HTTP %d (log id: %s)", res.StatusCode, res.Header.Get("X-Tt-Logid"))
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 {
		return status, fmt.Errorf("invalid voice clone response size")
	}
	var out responseBody
	if err := json.Unmarshal(raw, &out); err != nil {
		return status, fmt.Errorf("decode voice clone response: %w", err)
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
