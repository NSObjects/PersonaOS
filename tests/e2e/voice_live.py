"""Paid, real-provider acceptance against a disposable PersonaOS CI database."""

import base64
import http.cookiejar
import json
import os
import subprocess
import tempfile
import time
import urllib.error
import urllib.request
import uuid
from pathlib import Path

BASE = "http://127.0.0.1:9322"
SAMPLE = "我最喜欢的颜色是紫色。"
COOKIE_JAR = http.cookiejar.CookieJar()
CLIENT = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(COOKIE_JAR))


def request(method, path, payload=None, *, media_type="application/json",
            accept="application/json", expected=200):
    headers = {"Accept": accept}
    if payload is not None:
        headers["Content-Type"] = media_type
        if isinstance(payload, dict):
            payload = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    token = next((cookie.value for cookie in COOKIE_JAR
                  if cookie.name == "csrf_token"), None)
    if token:
        headers["X-CSRF-Token"] = token
    req = urllib.request.Request(BASE + path, data=payload, headers=headers, method=method)
    start = time.monotonic()
    try:
        with CLIENT.open(req, timeout=60) as response:
            assert response.status == expected, (path, response.status, expected)
            data = response.read()
            content_type = response.headers.get_content_type()
    except urllib.error.HTTPError as exc:
        # The request ID is enough to correlate the server's own diagnostics.
        raise RuntimeError(f"{method} {path}: HTTP {exc.code}, "
                           f"request_id={exc.headers.get('X-Request-ID', '')}") from None
    elapsed_ms = round((time.monotonic() - start) * 1000)
    return data, content_type, elapsed_ms


def api(method, path, payload=None, expected=200):
    data, content_type, elapsed = request(method, path, payload, expected=expected)
    assert content_type == "application/json", (path, content_type)
    envelope = json.loads(data)
    assert envelope["code"] == 100001, (path, envelope["code"])
    return envelope["data"], elapsed


def synthesize(device_id, text, output):
    data, content_type, elapsed = request(
        "POST", "/api/demo/tts", {"device_type_id": device_id, "text": text},
        accept="audio/ogg",
    )
    assert content_type == "audio/ogg", content_type
    verify_audio(data, output)
    return data, elapsed


def verify_audio(data, output):
    assert data.startswith(b"OggS") and b"OpusHead" in data[:1024]
    output.write_bytes(data)
    probe = subprocess.check_output([
        "ffprobe", "-v", "error", "-show_streams", "-of", "json", str(output),
    ], timeout=15)
    streams = json.loads(probe)["streams"]
    assert len(streams) == 1 and streams[0]["codec_name"] == "opus", streams
    # Decode the whole response, not only its container header.
    pcm = subprocess.check_output([
        "ffmpeg", "-v", "error", "-xerror", "-i", str(output),
        "-ac", "1", "-ar", "16000", "-f", "s16le", "pipe:1",
    ], timeout=15)
    assert len(pcm) >= 3200, "decoded TTS audio is empty or too short"



def stream_reply(payload, output):
    token = next(cookie.value for cookie in COOKIE_JAR if cookie.name == "csrf_token")
    req = urllib.request.Request(
        BASE + "/api/demo/chat", method="POST",
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers={"Accept": "application/x-ndjson", "Content-Type": "application/json",
                 "X-CSRF-Token": token},
    )
    start = time.monotonic()
    first_ms = None
    audio = bytearray()
    reply = None
    with CLIENT.open(req, timeout=60) as response:
        assert response.headers.get_content_type() == "application/x-ndjson"
        for line in response:
            event = json.loads(line)
            assert reply is None, "data after completion"
            if event["type"] == "audio":
                assert event["mime"] == "audio/ogg"
                chunk = base64.b64decode(event["audio"], validate=True)
                assert chunk
                if first_ms is None:
                    first_ms = round((time.monotonic() - start) * 1000)
                audio.extend(chunk)
            elif event["type"] == "done":
                reply = event["reply"]
            else:
                raise AssertionError("unexpected or failed voice event: " + event["type"])
    elapsed = round((time.monotonic() - start) * 1000)
    assert first_ms is not None and reply and reply["reply"].strip()
    verify_audio(audio, output)
    return reply, first_ms, elapsed


def memory_saved(character_id, session_id):
    # Only query the disposable database. Never expose a memory-management API
    # merely to inspect a test.
    container = os.environ["PERSONAOS_CI_MYSQL_CONTAINER"]
    query = (
        "SELECT IF((SELECT COUNT(*) FROM conversation_messages "
        f"WHERE character_id={int(character_id)} AND session_id='{uuid.UUID(session_id)}')=2 "
        "AND EXISTS(SELECT 1 FROM memory_facts "
        f"WHERE character_id={int(character_id)} AND value LIKE '%紫色%'), 1, 0)"
    )
    deadline = time.monotonic() + 25
    while time.monotonic() < deadline:
        result = subprocess.check_output([
            "docker", "exec", "-e", "MYSQL_PWD=ci-test-only", container,
            "mysql", "-u", "ci_user", "-D", "personaos_ci",
            "--default-character-set=utf8mb4", "-N", "-B", "-e", query,
        ], timeout=5)
        if result.strip() == b"1":
            return
        time.sleep(0.5)
    raise AssertionError("conversation or extracted color preference was not persisted")


def main():
    api("POST", "/api/auth/login", {
        "username": "ciadmin", "password": "ci-administrator-only",
    })
    character, _ = api("POST", "/api/characters", {
        "code": "live_voice_character", "name": "语音测试角色",
        "persona": "用中文友好自然地回答，每次只说一到两句简短的话。",
    }, expected=201)
    device, _ = api("POST", "/api/device-types", {
        "code": "live_voice_device", "name": "语音测试设备",
        "character_id": character["id"], "has_camera": False,
        "has_microphone": True, "has_speaker": True,
    }, expected=201)
    voice, _ = api("POST", "/api/capability-resources", {
        "kind": "voice", "code": "live_voice_resource", "name": "真实云端音色",
        "reference": os.environ["DASHSCOPE_TTS_VOICE"],
        "provider": "aliyun", "enabled": True,
    }, expected=201)
    device_id = device["id"]
    api("PUT", f"/api/device-types/{device_id}/capability-draft", {
        "voice_id": voice["id"], "tool_ids": [], "skill_ids": [], "knowledge_base_ids": [],
    })
    active, _ = api("PUT", f"/api/device-types/{device_id}/active-config", {})
    assert active["voice"]["id"] == voice["id"]

    with tempfile.TemporaryDirectory(prefix="personaos-voice-") as directory:
        # The test sentence is synthetic and contains no real personal data.
        # Generate an actual spoken fixture with the configured cloud TTS.
        fixture, fixture_ms = synthesize(device_id, SAMPLE, Path(directory) / "input.ogg")
        print(f"Fixture TTS: {fixture_ms} ms", flush=True)
        start = time.monotonic()
        data, content_type, asr_ms = request(
            "POST", "/api/demo/asr", fixture, media_type="audio/ogg;codecs=opus",
        )
        assert content_type == "application/json"
        result = json.loads(data)
        assert result["code"] == 100001
        transcript = result["data"]["text"]
        assert "紫色" in transcript, f"ASR did not recognize test phrase: {transcript!r}"
        session = str(uuid.uuid4())
        reply, first_ms, stream_ms = stream_reply({
            "device_type_id": device_id, "message": transcript, "history": [],
            "remember": True, "session_id": session,
        }, Path(directory) / "reply.ogg")
        assert reply["character_name"] == character["name"]
        assert reply["memory_queued"]
        total_ms = round((time.monotonic() - start) * 1000)
        print(f"ASR: {asr_ms} ms; reply first audio bytes: {first_ms} ms; "
              f"stream complete: {stream_ms} ms; ASR + first audio bytes: {asr_ms + first_ms} ms; "
              f"API chain with audio verification: {total_ms} ms", flush=True)

        memory_saved(character["id"], session)
        recalled, recall_ms = api("POST", "/api/demo/chat", {
            "device_type_id": device_id, "message": "我最喜欢什么颜色？",
            "history": [], "remember": True, "session_id": str(uuid.uuid4()),
        })
        assert "紫色" in recalled["reply"], "new session did not recall the preference"
        print(f"Memory recall in new session without history: {recall_ms} ms", flush=True)

    print("PASS: authenticated Opus ASR → streaming Eino → duplex Opus TTS; "
          "decoded audio and MySQL memory recall verified.", flush=True)


if __name__ == "__main__":
    main()
