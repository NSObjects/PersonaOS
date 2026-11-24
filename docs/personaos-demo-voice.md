# PersonaOS Voice Demo — Opus

The browser demo uses the existing `github.com/NSObjects/go-speech` SDK; PersonaOS does not implement vendor ASR/TTS protocols or add a voice orchestration framework.

## Push-to-talk path

1. `/agent-demo` records **native Opus** with `MediaRecorder`: Ogg Opus when supported, otherwise WebM/Opus. Browsers without either encoding report an unsupported format instead of pretending PCM is Opus.
2. `POST /api/demo/asr` accepts `audio/ogg;codecs=opus` or `audio/webm;codecs=opus` (maximum 1 MiB). It validates an Ogg Opus stream and its duration. For WebM/Opus, FFmpeg **remuxes without re-encoding** into Ogg Opus.
3. Aliyun and Volcengine ASR receive consecutive complete Ogg Opus pages through `go-speech`. Aliyun uses the nominal input sample rate from `OpusHead` (48 kHz when unspecified), while Volcengine retains its 16 kHz request; Tencent ASR does not accept the SDK's Ogg Opus format, so FFmpeg decodes the captured audio to PCM16LE, 16 kHz, mono.
4. Eino Agent uses the applied character persona, with optional memory.
5. `POST /api/demo/tts` synthesizes using the Voice resource in the **applied device-type configuration**. It uses **Ogg Opus** for Aliyun (24 kHz) and Volcengine (48 kHz). Tencent uses MP3 because its current SDK provider does not support Ogg Opus.
6. The browser requests Opus when `audio.canPlayType('audio/ogg; codecs="opus"')` reports support, and requests MP3 otherwise. The HTTP response carries the actual `Content-Type` (`audio/ogg` or `audio/mpeg`).

The client does not send a provider, secret, or Voice ID. Provider selection and voice reference are server-controlled.

## Deployment and secrets

**FFmpeg must be available on the PersonaOS server PATH** to remux WebM or decode for Tencent. The Docker runtime image includes FFmpeg. Direct Ogg Opus sent to Aliyun or Volcengine does not require FFmpeg. For local runs on macOS, `brew install ffmpeg` if using browsers that record WebM.

The LLM still uses `PERSONAOS_LLM_API_KEY`, `PERSONAOS_LLM_MODEL`, optional `PERSONAOS_LLM_BASE_URL`. Select ASR using `PERSONAOS_ASR_PROVIDER=aliyun|volcengine|tencent`. Set the corresponding server-only provider credentials:

| Provider | ASR settings | TTS settings |
|---|---|---|
| Volcengine | `VOLCENGINE_API_KEY`, optional `VOLCENGINE_ASR_RESOURCE_ID` | `VOLCENGINE_API_KEY`, optional `VOLCENGINE_TTS_RESOURCE_ID` |
| Aliyun | `DASHSCOPE_API_KEY`, optional `DASHSCOPE_WS_ENDPOINT` (SDK default when unset) | Same credentials and optional endpoint |
| Tencent | `TENCENT_APP_ID`, `TENCENT_SECRET_ID`, `TENCENT_SECRET_KEY` | Same credentials with a valid numeric voice ID |

In the capability resources admin interface, create a Voice resource using the provider key and exact vendor voice ID as the reference. Associate it with any dynamically created device type, then **应用配置**.

## Streaming voice reply API

`POST /api/demo/chat` accepts the usual chat payload with `Accept: application/x-ndjson`. Eino emits assistant text increments into one duplex TTS session while audio is received concurrently. The response contains newline-delimited JSON events:

- `{"type":"audio","mime":"audio/ogg","audio":"<base64 chunk>"}`: consecutive bytes of one audio stream, not independent playable files.
- `{"type":"done","reply":{...}}`: the ChatOutput fields, emitted after successful audio completion.
- `{"type":"error"}`: terminal failure after the response has started. Errors before the first chunk retain normal HTTP error handling.

A client must receive the done event before treating the turn as complete. The existing JSON chat response is for text-only chat. The browser still uses turn-based playback; incremental playback is not yet enabled.

## Scope and acceptance

- This is still **turn-based capture → ASR → Eino → TTS → complete browser playback**, not live microphone streaming, partial ASR rendering, TTS first-chunk playback, VAD or barge-in.
- Opus improves transport efficiency; it does not by itself make the Demo real-time. TTS SDK chunks are concatenated into a playable Ogg Opus stream.
- The server bounds input payload size, decoded length and output size, and preserves authentication, CSRF and `device_type:read` RBAC.
- Do not use real children's personal voice recordings during an unconsented Demo.
- Offline CI validates format selection, boundaries and SDK mocks. **Live cloud ASR/TTS requires paid credentials and must be smoke-tested separately**.

## Live acceptance (Aliyun)

The separate [Demo live voice workflow](../.github/workflows/demo-live-voice.yml) runs automatically on relevant pushes and can also be dispatched manually. It uses an ephemeral MySQL instance, a synthetic Mandarin test sentence, and the actual authenticated Demo APIs. It verifies Ogg Opus TTS output using FFmpeg, ASR recognition, the Eino reply, synthesized reply audio, and preference persistence and recall in a new session without chat history. The job summary records request durations for ASR, chat and TTS. This is a paid API test, not a physical microphone or browser autoplay test.

Repository configuration:

- Secrets: `DASHSCOPE_API_KEY`, `PERSONAOS_LLM_API_KEY`.
- Variables: `PERSONAOS_LLM_MODEL`; optional `PERSONAOS_LLM_BASE_URL`, `DASHSCOPE_TTS_VOICE` (defaults to `longanyang`), and `DASHSCOPE_WS_ENDPOINT` (only for a non-default endpoint). The LLM base URL and key must refer to the same compatible service.

Missing credentials fail the live job explicitly; the regular five-job CI remains independent. Use a fast non-reasoning model for this voice Demo.
