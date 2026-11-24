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

## Live Ogg Opus upload

For an Opus-capable ASR provider, the same authenticated `POST /api/demo/asr` accepts a continuous `audio/ogg;codecs=opus` request without a Content-Length (HTTP/1.1 chunked, or an HTTP/2 streaming body). It opens ASR on the first complete OpusHead and forwards each complete Ogg page immediately. It does not buffer the full recording or add replay pacing. The response is the final transcript when the input ends. Fixed-length upload remains available for providers that require PCM. Configure any reverse proxy not to buffer the streaming request.

The real CI sends a synthetic fixture in short Ogg pages paced at the client and reports capture-inclusive ASR time separately from upload-end to transcript time. The browser opens `GET /api/demo/asr/live?format=ogg|webm` after microphone permission and before capture starts. The same-origin authenticated WebSocket returns `ready`, accepts binary audio chunks and the text `end`, then returns the final transcript as `done`. For WebM Opus, FFmpeg remuxes incrementally without re-encoding; Ogg is forwarded directly. A provider requiring PCM returns `batch` before capture and uses the existing fixed-length upload. Reverse proxies must support WebSocket upgrades and preserve the original Host.

## Streaming voice reply API

`POST /api/demo/chat` accepts the usual chat payload with `Accept: application/x-ndjson`. Eino starts concurrently with the TTS handshake and emits assistant text increments into one duplex TTS session while audio is received concurrently. The response contains newline-delimited JSON events:

- `{"type":"audio","mime":"audio/ogg","audio":"<base64 chunk>"}`: consecutive bytes of one audio stream, not independent playable files.
- `{"type":"done","reply":{...}}`: the ChatOutput fields, emitted after successful audio completion.
- `{"type":"error"}`: terminal failure after the response has started. Errors before the first chunk retain normal HTTP error handling.

A client must receive the done event before treating the turn as complete. The existing JSON chat response is for text-only chat. The browser uses the streaming reply endpoint for Aliyun and Volcengine voices. It parses Ogg pages across HTTP chunks and decodes Opus packets with WebCodecs, scheduling audio through Web Audio without waiting for the complete response. The browser must support WebCodecs audio decoding. The existing Tencent MP3 playback remains unchanged.

## Scope and acceptance

- Opus-capable providers use live browser upload and streaming reply playback. Manual end-of-turn remains in use; VAD and automatic barge-in are not yet implemented.
- Opus improves transport efficiency; it does not by itself make the Demo real-time. TTS SDK chunks are concatenated into a playable Ogg Opus stream.
- The server bounds input payload size, decoded length and output size, and preserves authentication, CSRF and `device_type:read` RBAC.
- Do not use real children's personal voice recordings during an unconsented Demo.
- Offline CI validates format selection, boundaries and SDK mocks. **Live cloud ASR/TTS requires paid credentials and must be smoke-tested separately**.

## Live acceptance (Aliyun)

The separate [Demo live voice workflow](../.github/workflows/demo-live-voice.yml) runs automatically on relevant pushes and can also be dispatched manually. It uses an ephemeral MySQL instance, a synthetic Mandarin test sentence, and the actual authenticated Demo APIs. It verifies Ogg Opus TTS output using FFmpeg, ASR recognition, the Eino reply, synthesized reply audio, and preference persistence and recall in a new session without chat history. The job summary records ASR request duration, first received reply audio bytes and total streamed reply duration, plus server timings for the TTS handshake, first LLM text, first sent TTS text and first received audio bytes. These are transport diagnostics, not first audible speech. The deployment latency target is under 1 second from end of user speech to first audible reply, including endpointing, provider calls and device playback; measure it on the actual deployment network and device, not a remote CI runner. This is a paid API test, not a physical microphone or browser autoplay test.

Repository configuration:

- Secrets: `DASHSCOPE_API_KEY`, `PERSONAOS_LLM_API_KEY`.
- Variables: `PERSONAOS_LLM_MODEL`; optional `PERSONAOS_LLM_BASE_URL`, `DASHSCOPE_TTS_VOICE` (defaults to `longanyang`), and `DASHSCOPE_WS_ENDPOINT` (only for a non-default endpoint). The LLM base URL and key must refer to the same compatible service.

Missing credentials fail the live job explicitly; the regular five-job CI remains independent. Use a fast non-reasoning model for this voice Demo.

## 实时执行轨迹

角色对话页的「执行轨迹」视图与对话共用本轮 NDJSON 流；记录表、时间概览和检查器由事件实时投影。通过 `POST /api/demo/chat?trajectory=1&voice=0`（`voice=1` 为双工语音回复）与 `Accept: application/x-ndjson` 获取 `trace`、`reply_preview`（累积文字预览）、`audio`（语音模式）、`done` 或 `error` 帧。每条执行记录以本轮 invocation ID 配对 start/end，时间单位为 Unix 毫秒。缺失的耗时及 Token 不估算。队列饱和时通过 `trace_gap` 标明观测不完整。工具参数和输出仅保留有界文本，不采集模型系统提示词、供应商凭证或原始音频到轨迹记录；轨迹在单进程 Session 中有界保留，可独立订阅，无持久化。模型输入目前展示消息及工具数量。

### 执行耗时

- 流式模型与双工 TTS 的首输出通过同一 invocation 的 `first_output` 事件记录，首输出等待为该调用开始至第一段有效输出的实测时间，不是供应商报告的精确首 Token 时间。非流式模型没有此指标。
- Memory 记录实际偏好读取的成功或失败及事实数量；RAG 记录实际 Redis 检索及命中数；Skill 记录实际加载及名称。未触发的阶段不产生记录，后台记忆提取不算作本轮同步执行。
- ASR 结束等待和非双工 TTS 使用浏览器单调时钟实测完整等待时间，包括客户端和网络开销，不加入服务端时间轴，不代表云厂商的纯处理耗时。
