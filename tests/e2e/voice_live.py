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



def live_asr(fixture):
    # Remux to short Ogg pages without re-encoding. Replay those pages at their
    # granule clock on the client, rather than pacing an already uploaded file
    # on the server. The HTTP iterable is sent with chunked transfer encoding.
    ogg = subprocess.check_output([
        "ffmpeg", "-v", "error", "-f", "ogg", "-i", "pipe:0",
        "-c:a", "copy", "-page_duration", "20000", "-f", "ogg", "pipe:1",
    ], input=fixture, timeout=15)
    finished = None

    def pages():
        nonlocal finished
        start = time.monotonic()
        pos = 0
        while pos < len(ogg):
            assert ogg[pos:pos + 4] == b"OggS"
            count = ogg[pos + 26]
            end = pos + 27 + count + sum(ogg[pos + 27:pos + 27 + count])
            granule = int.from_bytes(ogg[pos + 6:pos + 14], "little")
            if granule != (1 << 64) - 1:
                time.sleep(max(0, start + granule / 48000 - time.monotonic()))
            yield ogg[pos:end]
            pos = end
        finished = time.monotonic()

    data, content_type, total_ms = request(
        "POST", "/api/demo/asr", pages(), media_type="audio/ogg;codecs=opus",
    )
    assert finished is not None
    tail_ms = round((time.monotonic() - finished) * 1000)
    assert content_type == "application/json"
    result = json.loads(data)
    assert result["code"] == 100001
    return result["data"]["text"], total_ms, tail_ms


def stream_reply(payload, output):
    token = next(cookie.value for cookie in COOKIE_JAR if cookie.name == "csrf_token")
    req = urllib.request.Request(
        BASE + "/api/demo/chat?trajectory=1&voice=1", method="POST",
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers={"Accept": "application/x-ndjson", "Content-Type": "application/json",
                 "X-CSRF-Token": token},
    )
    start = time.monotonic()
    first_ms = None
    audio = bytearray()
    reply = None
    preview = False
    traces = []
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
            elif event["type"] == "reply_preview":
                assert event["text"].strip()
                preview = True
            elif event["type"] == "trace":
                traces.append(event["event"])
            elif event["type"] == "done":
                reply = event["reply"]
            else:
                raise AssertionError("unexpected or failed voice event: " + event["type"])
    elapsed = round((time.monotonic() - start) * 1000)
    assert first_ms is not None and reply and reply["reply"].strip()
    assert preview, "streaming assistant text not observed"
    assert any(item["kind"] == "model" and item["phase"] == "first_output" for item in traces), traces
    assert any(item["kind"] == "audio" and item["phase"] == "first_output" for item in traces), traces
    batch, _ = api("GET", f"/api/demo/sessions/{payload['session_id']}/events?after=0")
    events = batch["events"]
    assert not batch["gap"] and events[0]["type"] == "turn_start", batch
    assert any(item["type"] == "reply_preview" for item in events), events
    assert events[-1]["type"] == "turn_done" and events[-1]["reply"] == reply, events[-1]
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
        transcript, asr_ms, tail_ms = live_asr(fixture)
        assert "紫色" in transcript, f"ASR did not recognize test phrase: {transcript!r}"
        session = str(uuid.uuid4())
        reply, first_ms, stream_ms = stream_reply({
            "device_type_id": device_id, "message": transcript, "history": [],
            "remember": True, "session_id": session,
        }, Path(directory) / "reply.ogg")
        assert reply["character_name"] == character["name"]
        assert reply["memory_queued"]
        total_ms = round((time.monotonic() - start) * 1000)
        print(f"Live ASR: {asr_ms} ms (including capture); ASR tail: {tail_ms} ms; "
              f"reply first audio bytes: {first_ms} ms; "
              f"stream complete: {stream_ms} ms; end of audio upload + first reply bytes: {tail_ms + first_ms} ms; "
              f"API chain with audio verification: {total_ms} ms", flush=True)

        memory_saved(character["id"], session)
        recalled, recall_ms = api("POST", "/api/demo/chat", {
            "device_type_id": device_id, "message": "我最喜欢什么颜色？",
            "history": [], "remember": True, "session_id": str(uuid.uuid4()),
        })
        assert "紫色" in recalled["reply"], f"new session did not recall the preference: {recalled['reply']!r}"
        print(f"Memory recall in new session without history: {recall_ms} ms", flush=True)

    knowledge, _ = api("POST", "/api/capability-resources", {
        "kind": "knowledge_base", "code": "live_voice_lore", "name": "角色专属知识",
        "provider": "redis", "enabled": True,
        "knowledge_filename": "lore.md", "knowledge_text": "月光徽章的编号是 LX-7429。",
    }, expected=201)
    assert knowledge["reference"].startswith("kb-")
    status, _ = api("GET", f"/api/capability-resources/{knowledge['id']}/knowledge-status")
    assert status["state"] == "indexed" and status["chunks"] > 0, status
    preview, _ = api("POST", f"/api/capability-resources/{knowledge['id']}/knowledge-search", {"query": "月光徽章的编号"})
    assert preview["version"] == knowledge["reference"]
    assert any("LX-7429" in item["content"] for item in preview["hits"]), preview
    print("PASS: real index inspection and direct knowledge retrieval", flush=True)
    api("PUT", f"/api/device-types/{device_id}/capability-draft", {
        "voice_id": voice["id"], "tool_ids": [], "skill_ids": [],
        "knowledge_base_ids": [knowledge["id"]],
    })
    api("PUT", f"/api/device-types/{device_id}/active-config", {})
    grounded, _ = api("POST", "/api/demo/chat", {
        "device_type_id": device_id, "message": "请检索专属知识库，月光徽章的编号是什么？",
        "history": [],
    })
    assert "LX-7429" in grounded["reply"], "knowledge retrieval did not ground the answer"
    print("PASS: uploaded Redis knowledge retrieved by real Eino Agent", flush=True)
    updated, _ = api("PATCH", f"/api/capability-resources/{knowledge['id']}", {
        "name": knowledge["name"], "provider": "redis", "enabled": True,
        "knowledge_filename": "lore-v2.md", "knowledge_text": "月光徽章的新编号是 LX-8530。",
    })
    assert updated["reference"] != knowledge["reference"]
    snapshot, _ = api("GET", f"/api/device-types/{device_id}/active-config")
    assert snapshot["knowledge_bases"][0]["reference"] == knowledge["reference"]
    published, _ = api("PUT", f"/api/device-types/{device_id}/active-config", {})
    assert published["knowledge_bases"][0]["reference"] == updated["reference"]
    print("PASS: document upload creates new immutable version; active snapshot changes only on apply", flush=True)

    configured_tool, _ = api("POST", "/api/capability-resources", {
        "kind": "tool", "code": "query_device_badge", "name": "徽章编号查询",
        "description": "查询设备专属徽章的真实编号，用户询问时必须调用。",
        "reference": "ci_badge", "provider": "http", "enabled": True,
        "tool_config": json.dumps({"method": "GET", "input_schema": {"type": "object", "properties": {}}}),
    }, expected=201)
    api("PUT", f"/api/device-types/{device_id}/capability-draft", {
        "voice_id": voice["id"], "tool_ids": [configured_tool["id"]],
        "skill_ids": [], "knowledge_base_ids": [],
    })
    published, _ = api("PUT", f"/api/device-types/{device_id}/active-config", {})
    assert published["tools"][0]["tool_config"] == configured_tool["tool_config"]
    result, _ = api("POST", "/api/demo/chat", {
        "device_type_id": device_id, "message": "请调用徽章编号查询工具，告诉我设备专属徽章的编号。", "history": [],
    })
    assert "HT-8396" in result["reply"], "configured HTTP tool was not used by the real Agent"
    print("PASS: published HTTP tool executed by real Eino Agent", flush=True)

    print("PASS: authenticated Opus ASR → streaming Eino → duplex Opus TTS; "
          "decoded audio and MySQL memory recall verified.", flush=True)


if __name__ == "__main__":
    main()
