"""D1 real-browser acceptance against Go + MySQL + the Umi admin frontend.

This is intentionally not a mocked component test: the browser authenticates,
calls real API routes and verifies persisted character/device-type configuration.
Run only against a disposable D1 test database.
"""

import json
import os
import re
import uuid
from pathlib import Path
from urllib.parse import unquote

from playwright.sync_api import expect, sync_playwright

BASE = os.environ.get("PERSONAOS_E2E_BASE_URL", "http://127.0.0.1:8000")
USER = os.environ.get("PERSONAOS_E2E_USERNAME", "ciadmin")
PASSWORD = os.environ.get("PERSONAOS_E2E_PASSWORD", "ci-administrator-only")
ARTIFACT_DIR = Path(os.environ.get("PERSONAOS_E2E_ARTIFACT_DIR", "/tmp/personaos-e2e"))


def save(page, name):
    ARTIFACT_DIR.mkdir(parents=True, exist_ok=True)
    page.screenshot(path=str(ARTIFACT_DIR / name), full_page=True)


def submit_modal(modal):
    modal.locator("button.ant-btn-primary").last.click()
    expect(modal).not_to_be_visible(timeout=15000)


def populate_device(page, code, role_name, camera):
    page.goto(BASE + "/device-types", wait_until="domcontentloaded")
    expect(page.get_by_role("button", name="新增设备类型")).to_be_visible(timeout=30000)
    page.get_by_role("button", name="新增设备类型").click()
    modal = page.get_by_role("dialog")
    expect(modal).to_be_visible()
    modal.get_by_label("型号编码").fill(code)
    modal.get_by_label("型号名称").fill(code + " 测试型号")
    # Click the accessible input; it overlays the Select placeholder in Ant v6.
    modal.get_by_role("combobox").click()
    page.get_by_text(role_name, exact=True).last.click()
    if camera:
        modal.locator("button[role='switch']").first.click()
    submit_modal(modal)
    expect(page.get_by_role("row").filter(has_text=code)).to_be_visible(timeout=15000)


def delete_row(page, code):
    row = page.get_by_role("row").filter(has_text=code)
    expect(row).to_be_visible()
    row.get_by_role("button", name="删除").click()
    page.locator(".ant-popconfirm:visible button.ant-btn-primary").click()
    expect(row).not_to_be_visible(timeout=15000)


def verify_session_observer(context, device_type_id):
    """Exercise the real session API without a paid LLM or fake assistant reply."""
    session_id = str(uuid.uuid4())
    csrf = next(cookie["value"] for cookie in context.cookies() if cookie["name"] == "csrf_token")

    def send(text):
        response = context.request.post(
            BASE + "/api/demo/chat?trajectory=1&voice=0",
            headers={"Accept": "application/x-ndjson", "X-CSRF-Token": unquote(csrf)},
            data={"device_type_id": device_type_id, "session_id": session_id, "message": text},
        )
        assert response.ok, f"session sender returned {response.status}"
        # This device has not applied configuration, so no paid model is called.
        frames = [json.loads(line) for line in response.text().splitlines()]
        assert frames[-1]["type"] == "error", frames

    first, second, third = "观测窗口第一轮", "观测窗口实时同步", "断开期间的消息"
    send(first)
    observer = context.new_page()
    try:
        observer.goto(BASE + "/agent-demo?observe=" + session_id, wait_until="domcontentloaded")
        expect(observer.get_by_text("只读会话观测", exact=True)).to_be_visible(timeout=30000)
        expect(observer.get_by_text(first, exact=True)).to_be_visible(timeout=30000)
        expect(observer.get_by_role("button", name="按键录音")).to_have_count(0)
        send(second)
        expect(observer.get_by_text(second, exact=True)).to_be_visible(timeout=30000)

        observer.get_by_role("button", name="断开观测").click()
        expect(observer.get_by_role("button", name="重新连接")).to_be_visible()
        send(third)
        expect(observer.get_by_text(third, exact=True)).to_have_count(0)
        observer.get_by_role("button", name="重新连接").click()
        expect(observer.get_by_text(third, exact=True)).to_be_visible(timeout=30000)
        expect(observer.get_by_text(first, exact=True)).to_have_count(1)
        expect(observer.get_by_text(second, exact=True)).to_have_count(1)
        expect(observer.get_by_role("tab", name="执行轨迹")).to_be_visible()
    finally:
        observer.close()


def main():
    character_code = "d1_browser_role"
    role_name = "小新浏览器验收"
    new_name = "小新浏览器验收（已修改）"
    a3 = "A3_BROWSER"
    a4 = "A4_BROWSER"
    ARTIFACT_DIR.mkdir(parents=True, exist_ok=True)

    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True, args=["--no-sandbox"])
        context = browser.new_context(viewport={"width": 1440, "height": 960})
        page = context.new_page()
        page.set_default_timeout(15000)
        try:
            page.goto(BASE + "/user/login", wait_until="domcontentloaded")
            expect(page.get_by_placeholder("请输入用户名")).to_be_visible(timeout=45000)
            page.get_by_placeholder("请输入用户名").fill(USER)
            page.get_by_placeholder("请输入密码").fill(PASSWORD)
            page.locator(".ant-pro-form-login-main button.ant-btn-primary").click()
            expect(page).to_have_url(re.compile(r"/dashboard"), timeout=30000)

            # Verify the actual granted sidebar entry, not only direct URL routing.
            group = page.get_by_text("AI角色平台", exact=True)
            expect(group).to_be_visible(timeout=30000)
            entry = page.get_by_text("AI 对话演示", exact=True)
            if not entry.is_visible():
                group.click()
            entry.click()
            expect(page).to_have_url(re.compile(r"/agent-demo"), timeout=30000)
            expect(page.get_by_role("tab", name="执行轨迹")).to_be_visible(timeout=30000)

            page.goto(BASE + "/characters", wait_until="domcontentloaded")
            expect(page.get_by_role("button", name="新增角色")).to_be_visible(timeout=30000)
            page.get_by_role("button", name="新增角色").click()
            modal = page.get_by_role("dialog")
            expect(modal).to_be_visible()
            modal.get_by_label("角色编码").fill(character_code)
            modal.get_by_label("名称", exact=True).fill(role_name)
            modal.get_by_label("人格设定").fill("活泼、好奇、喜欢与朋友聊天")
            submit_modal(modal)
            expect(page.get_by_role("row").filter(has_text=character_code)).to_be_visible(timeout=15000)

            populate_device(page, a3, role_name, False)
            populate_device(page, a4, role_name, True)

            # Validate that both actual records point to exactly one role ID.
            response = context.request.get(BASE + "/api/device-types")
            assert response.ok, f"device-type API returned {response.status}"
            payload = response.json()
            models = {item["code"]: item for item in payload["data"]}
            assert models[a3]["character_id"] == models[a4]["character_id"], models
            assert models[a3]["has_camera"] is False, models[a3]
            assert models[a4]["has_camera"] is True, models[a4]
            role_id = models[a3]["character_id"]
            verify_session_observer(context, models[a3]["id"])

            # Update the shared persona; no role copy is created.
            page.goto(BASE + "/characters", wait_until="domcontentloaded")
            row = page.get_by_role("row").filter(has_text=character_code)
            expect(row).to_be_visible(timeout=15000)
            row.get_by_text("编辑", exact=True).click()
            modal = page.get_by_role("dialog")
            modal.get_by_label("名称", exact=True).fill(new_name)
            submit_modal(modal)
            expect(page.get_by_role("row").filter(has_text=new_name)).to_be_visible()
            response = context.request.get(BASE + "/api/characters")
            assert response.ok
            records = [item for item in response.json()["data"] if item["id"] == role_id]
            assert len(records) == 1 and records[0]["name"] == new_name, records

            # Reference protection must reject UI deletion and keep the role.
            row = page.get_by_role("row").filter(has_text=character_code)
            row.get_by_role("button", name="删除").click()
            with page.expect_response(
                lambda res: res.request.method == "DELETE"
                and "/api/characters/" in res.url,
                timeout=15000,
            ) as deletion:
                page.locator(".ant-popconfirm:visible button.ant-btn-primary").click()
            assert deletion.value.status == 409, deletion.value.status
            expect(page.get_by_role("row").filter(has_text=character_code)).to_be_visible()

            # Cleanup through the actual management UI.
            page.goto(BASE + "/device-types", wait_until="domcontentloaded")
            expect(page.get_by_role("row").filter(has_text=a3)).to_be_visible(timeout=15000)
            delete_row(page, a3)
            delete_row(page, a4)
            page.goto(BASE + "/characters", wait_until="domcontentloaded")
            expect(page.get_by_role("row").filter(has_text=character_code)).to_be_visible(timeout=15000)
            delete_row(page, character_code)

            response = context.request.get(BASE + "/api/characters")
            assert response.ok
            assert all(item["code"] != character_code for item in response.json()["data"])
            print("D1 browser acceptance passed: login, CRUD, A3/A4 sharing, FK protection")
        except Exception:
            save(page, "d1-browser-failure.png")
            raise
        finally:
            context.close()
            browser.close()


if __name__ == "__main__":
    main()
