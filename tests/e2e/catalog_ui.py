"""D1 real-browser acceptance against Go + MySQL + the Umi admin frontend.

This is intentionally not a mocked component test: the browser authenticates,
calls real API routes and verifies persisted character/device-type configuration.
Run only against a disposable D1 test database.
"""

import os
import re
from pathlib import Path

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
    modal.locator(".ant-select-selector").click()
    page.locator(".ant-select-dropdown:visible").get_by_text(role_name, exact=True).click()
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
            page.get_by_role("button", name="登录", exact=True).click()
            expect(page).to_have_url(re.compile(r"/dashboard"), timeout=30000)

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
