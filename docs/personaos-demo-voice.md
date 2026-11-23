# PersonaOS Voice Demo

Use the existing `github.com/NSObjects/go-speech` SDK rather than writing cloud ASR/TTS transports.

## Shortest browser voice turn

`/agent-demo` lets an administrator choose any device type previously created and applied in the backend:

1. Click **按键录音**, speak for up to 15 seconds, and click **结束录音并发送**.
2. Browser microphone is captured and resampled to signed **PCM16LE, 16 kHz, mono**. Audio is sent only to `POST /api/demo/asr` after capture stops.
3. Go forwards 40 ms frames through `speech.NewASR(...).OpenStream`; final ASR text becomes the existing Eino chat request, including opt-in memory if enabled.
4. `POST /api/demo/tts` looks up the **confirmed device-type snapshot**, selects the saved Voice provider and Reference (vendor voice ID), and calls `speech.NewTTS(...).SynthesizeStream` with MP3/24 kHz/mono.
5. Browser receives a complete MP3 response and presents an audio player. Browsers can block automatic playback, so the player has manual controls.

Both voice endpoints are managed, authenticated admin APIs protected by existing CSRF and `device_type:read` authorization, **not** public hardware APIs. Input is capped at 15 seconds. The SDK is not asked to transcode; exact audio formats are enforced at both boundaries. Recorded PCM is not persisted in PersonaOS.

## Server configuration

The LLM still uses `PERSONAOS_LLM_API_KEY`, `PERSONAOS_LLM_MODEL` and optional `PERSONAOS_LLM_BASE_URL`.

Select **one ASR provider** by `PERSONAOS_ASR_PROVIDER=volcengine|aliyun|tencent`. Leave unset to keep text chat enabled without ASR. Provider credentials remain on the server:

| Provider | ASR settings | TTS settings |
|---|---|---|
| Volcengine | `VOLCENGINE_API_KEY`, optionally `VOLCENGINE_ASR_RESOURCE_ID` (defaults to `volc.seedasr.sauc.duration`) | `VOLCENGINE_API_KEY`, optionally `VOLCENGINE_TTS_RESOURCE_ID` (defaults to `seed-tts-2.0`) |
| Aliyun | `DASHSCOPE_API_KEY`, `DASHSCOPE_WS_ENDPOINT` | Same credential and workspace endpoint |
| Tencent | `TENCENT_APP_ID`, `TENCENT_SECRET_ID`, `TENCENT_SECRET_KEY` | Same credentials, with an account-entitled numeric voice ID |

Under **能力资源**, create one `voice` resource: `provider` must be `aliyun`, `volcengine`, or `tencent`; `reference` is the **exact provider voice ID**. Bind the voice to a device type, save its draft, and click **应用配置**. There is no voice ID in the synthesis request from the browser.

The Tencent TTS SDK adapter has local protocol tests but no claimed successful account-entitled live synthesis; test that provider with actual service credentials before demonstrating it.

## Verified boundaries

- This is **push-to-talk and turn-based**. Recording is not streamed to the server during speech; the first Demo does not implement VAD, echo cancellation on the server, barge-in, real-time ASR partial updates, or true chunk-by-chunk audio playback.
- The ASR SDK handles provider streaming sessions; the browser uploads a complete recording. TTS SDK streaming chunks are aggregated to one MP3 for browser compatibility.
- Recording uses the browser microphone and requires HTTPS or localhost with microphone permission.
- Do not use real children’s voice recordings in this internal Demo without consent and a defined retention policy for third-party cloud services.
- External provider credentials are required for a real audio smoke test. Green offline CI does not establish that the provider has billing entitlement, a compatible voice/model or low enough latency.

This work needs Go 1.27.4 because the current `go-speech` SDK module explicitly requires it.
