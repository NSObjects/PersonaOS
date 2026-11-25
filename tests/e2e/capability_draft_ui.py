"""D2 browser acceptance: independent draft capabilities on shared characters.

Runs against the disposable Go + MySQL + Umi stack started by CI.
All resources are created through authenticated management API calls, while
the actual draft selections are saved and reloaded through the browser UI.
"""

import json
import os
import re
from pathlib import Path

from playwright.sync_api import expect, sync_playwright

BASE = os.environ.get("PERSONAOS_E2E_BASE_URL", "http://127.0.0.1:8000")
USER = os.environ.get("PERSONAOS_E2E_USERNAME", "ciadmin")
PASSWORD = os.environ.get("PERSONAOS_E2E_PASSWORD", "ci-administrator-only")
ARTIFACT_DIR = Path(os.environ.get("PERSONAOS_E2E_ARTIFACT_DIR", "/tmp/personaos-e2e"))


def csrf_headers(request):
    # APIRequestContext shares the browser cookie jar, but unlike the Umi
    # interceptor it does not automatically echo the double-submit token.
    cookies = request.storage_state().get("cookies", [])
    token = next((cookie["value"] for cookie in cookies
                  if cookie["name"] == "csrf_token"), None)
    assert token, "authenticated browser context has no csrf_token cookie"
    return {"Content-Type": "application/json", "X-CSRF-Token": token}


def api_create(request, endpoint, payload):
    response = request.post(
        BASE + endpoint,
        data=json.dumps(payload),
        headers=csrf_headers(request),
    )
    assert response.status == 201, (endpoint, response.status, response.text())
    return response.json()["data"]


def api_get(request, endpoint):
    response = request.get(BASE + endpoint)
    assert response.ok, (endpoint, response.status, response.text())
    return response.json()["data"]


def create_device(request, code, character_id, camera):
    return api_create(request, "/api/device-types", {
        "code": code,
        "name": code,
        "description": "D2 disposable browser acceptance",
        "character_id": character_id,
        "has_camera": camera,
        "has_microphone": True,
        "has_speaker": True,
    })


def create_resource(request, kind, code, name, reference, provider=""):
    return api_create(request, "/api/capability-resources", {
        "kind": kind,
        "code": code,
        "name": name,
        "reference": reference,
        "provider": provider,
        "description": "D2 browser acceptance",
        "enabled": True,
    })


def choose_resource(page, modal, index, label):
    modal.get_by_role("combobox").nth(index).click()
    page.get_by_text(label, exact=True).last.click()


def save_draft(page, code, selections):
    row = page.get_by_role("row").filter(has_text=code)
    expect(row).to_be_visible(timeout=30000)
    row.get_by_text("能力草稿", exact=True).click()
    modal = page.get_by_role("dialog")
    expect(modal).to_be_visible()
    for index, label in selections:
        choose_resource(page, modal, index, label)
    modal.get_by_role("button", name="保存草稿").click()
    expect(modal).not_to_be_visible(timeout=20000)


def assert_draft(request, device, voice_id, tool_ids, skill_ids, knowledge_ids):
    draft = api_get(request, f"/api/device-types/{device['id']}/capability-draft")
    assert draft["device_type_id"] == device["id"], draft
    assert draft["voice_id"] == voice_id, draft
    assert sorted(draft.get("tool_ids") or []) == sorted(tool_ids), draft
    assert sorted(draft.get("skill_ids") or []) == sorted(skill_ids), draft
    assert sorted(draft.get("knowledge_base_ids") or []) == sorted(knowledge_ids), draft
    return draft


def assert_status(request, method, path, status, data=None):
    response = request.fetch(
        BASE + path,
        method=method,
        data=json.dumps(data) if data is not None else None,
        headers=csrf_headers(request) if method in ("POST", "PUT", "PATCH", "DELETE") else None,
    )
    assert response.status == status, (method, path, response.status, response.text())


def create_installed_resource(page, request, kind, reference, label):
    page.goto(BASE + "/capability-resources", wait_until="domcontentloaded")
    expect(page.get_by_role("button", name="新增资源")).to_be_visible(timeout=30000)
    page.get_by_role("button", name="新增资源").click()
    modal = page.get_by_role("dialog")
    expect(modal).to_be_visible()
    modal.get_by_role("combobox").first.click()
    page.locator(".ant-select-dropdown:visible").get_by_text(kind, exact=True).click()
    modal.get_by_role("combobox").last.click()
    page.locator(".ant-select-dropdown:visible").get_by_text(label, exact=True).click()
    expect(modal.get_by_label("唯一编码")).to_have_value(reference)
    expect(modal.get_by_label("资源名称")).not_to_have_value("")
    modal.locator("button.ant-btn-primary").last.click()
    expect(modal).not_to_be_visible(timeout=20000)
    resource_kind = kind.lower()
    records = api_get(request, f"/api/capability-resources?kind={resource_kind}")
    matches = [item for item in records if item["reference"] == reference]
    assert len(matches) == 1, matches
    return matches[0]


def main():
    ARTIFACT_DIR.mkdir(parents=True, exist_ok=True)
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True, args=["--no-sandbox"])
        context = browser.new_context(viewport={"width": 1440, "height": 960})
        page = context.new_page()
        page.set_default_timeout(15000)
        request = context.request
        try:
            page.goto(BASE + "/user/login", wait_until="domcontentloaded")
            expect(page.get_by_placeholder("请输入用户名")).to_be_visible(timeout=45000)
            page.get_by_placeholder("请输入用户名").fill(USER)
            page.get_by_placeholder("请输入密码").fill(PASSWORD)
            page.locator(".ant-pro-form-login-main button.ant-btn-primary").click()
            expect(page).to_have_url(re.compile(r"/dashboard"), timeout=30000)

            character = api_create(request, "/api/characters", {
                "code": "d2_browser_role",
                "name": "D2 shared character",
                "persona": "Friendly and helpful",
            })
            a3 = create_device(request, "A3_D2_BROWSER", character["id"], False)
            a4 = create_device(request, "A4_D2_BROWSER", character["id"], True)
            voice = create_resource(request, "voice", "d2_browser_voice", "D2 Voice",
                                    "voice.d2", provider="aliyun")
            options = api_get(request, "/api/demo/resource-options")
            assert any(item["reference"] == "get_current_time" for item in options["tools"]), options
            assert any(item["reference"] == "storytelling" for item in options["skills"]), options
            tool = create_installed_resource(page, request, "Tool", "get_current_time", "当前时间 · get_current_time")
            assert tool["provider"] == "builtin", tool
            skill = create_installed_resource(page, request, "Skill", "storytelling", "故事讲述 · storytelling")
            knowledge = create_resource(request, "knowledge_base", "d2_browser_kb",
                                        "D2 Knowledge", "indexes.d2")

            page.goto(BASE + "/device-types", wait_until="domcontentloaded")
            save_draft(page, a3["code"], [(0, "D2 Voice (d2_browser_voice)")])
            save_draft(page, a4["code"], [
                (1, f"{tool['name']} ({tool['code']})"),
                (2, f"{skill['name']} ({skill['code']})"),
                (3, "D2 Knowledge (d2_browser_kb)"),
            ])

            assert_draft(request, a3, voice["id"], [], [], [])
            assert_draft(request, a4, 0, [tool["id"]], [skill["id"]], [knowledge["id"]])

            # Runtime-facing configuration must not exist until explicit apply.
            assert_status(request, "GET", f"/api/device-types/{a3['id']}/active-config", 404)
            for device in [a3, a4]:
                row = page.get_by_role("row").filter(has_text=device["code"])
                row.get_by_role("button", name="应用配置").click()
                page.locator(".ant-popover:visible").filter(has_text="应用当前草稿到 Demo？").locator(".ant-popconfirm-buttons .ant-btn-primary").click()
                confirmed = page.get_by_role("dialog")
                expect(confirmed.get_by_text("已生效", exact=True)).to_be_visible()
                confirmed.locator(".ant-modal-close").click()
                expect(confirmed).not_to_be_visible()

            active_a3 = api_get(request, f"/api/device-types/{a3['id']}/active-config")
            active_a4 = api_get(request, f"/api/device-types/{a4['id']}/active-config")
            assert active_a3["version"] == active_a4["version"] == 1
            assert active_a3["character"]["id"] == active_a4["character"]["id"]
            assert active_a3["voice"]["id"] == voice["id"]
            assert active_a4.get("voice") is None
            assert [item["id"] for item in active_a4["tools"]] == [tool["id"]]

            # Applied configuration links to the exact selected device model.
            row = page.get_by_role("row").filter(has_text=a4["code"])
            row.get_by_role("button", name="生效配置").click()
            modal = page.get_by_role("dialog")
            modal.get_by_role("link", name="开始对话测试").click()
            expect(page).to_have_url(re.compile(r"/agent-demo\?device_type_id=" + str(a4["id"])), timeout=30000)
            expect(page.get_by_text("已应用 · D2 shared character · V1", exact=True)).to_be_visible(timeout=30000)
            expect(page.get_by_text("服务配置", exact=True)).to_be_visible()
            expect(page.get_by_text(re.compile(r"LLM · (已配置|未配置)"))).to_be_visible(timeout=30000)
            page.get_by_role("button", name="新建会话").click()
            expect(page.get_by_text("已应用 · D2 shared character · V1", exact=True)).to_be_visible(timeout=30000)

            # A browser reload must preserve the persisted A3 selection.
            page.goto(BASE + "/device-types", wait_until="domcontentloaded")
            row = page.get_by_role("row").filter(has_text=a3["code"])
            expect(row).to_be_visible(timeout=30000)
            row.get_by_text("能力草稿", exact=True).click()
            modal = page.get_by_role("dialog")
            expect(modal.get_by_text("D2 Voice (d2_browser_voice)")).to_be_visible()
            modal.locator(".ant-modal-close").click()
            expect(modal).not_to_be_visible(timeout=10000)

            # The database must reject removal of resources still in use.
            assert_status(request, "DELETE", f"/api/capability-resources/{voice['id']}", 409)

            # A mismatched kind cannot overwrite a valid draft.
            assert_status(request, "PUT", f"/api/device-types/{a3['id']}/capability-draft",
                          400, {
                              "voice_id": voice["id"],
                              "tool_ids": [voice["id"]],
                              "skill_ids": [],
                              "knowledge_base_ids": [],
                          })
            assert_draft(request, a3, voice["id"], [], [], [])

            # Cleanup in dependency order, also covering clearing a draft.
            empty = {"voice_id": 0, "tool_ids": [], "skill_ids": [], "knowledge_base_ids": []}
            for device in [a3, a4]:
                assert_status(request, "PUT",
                              f"/api/device-types/{device['id']}/capability-draft", 200, empty)
            for resource in [voice, tool, skill, knowledge]:
                assert_status(request, "DELETE",
                              f"/api/capability-resources/{resource['id']}", 200)
            for device in [a3, a4]:
                assert_status(request, "DELETE", f"/api/device-types/{device['id']}", 200)
            assert_status(request, "DELETE", f"/api/characters/{character['id']}", 200)
            print("D2 browser acceptance passed: A3/A4 independent drafts, reload, FK, rollback")
        except Exception:
            page.screenshot(path=str(ARTIFACT_DIR / "d2-browser-failure.png"), full_page=True)
            raise
        finally:
            context.close()
            browser.close()


if __name__ == "__main__":
    main()
