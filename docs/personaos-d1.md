# PersonaOS D1：角色资源与设备类型

本迭代基于仓库现有 Echo v5 / MySQL / Umi Max 框架完成，不使用旧的独立 D1 示例应用。

## 边界

- 角色资源是可被多个设备类型共享的人格定义（编码、名称、人格、说明）。
- 设备类型（例如 A3、A4）引用相同的 `character_id`，但拥有不同物理能力（摄像头、麦克风、扬声器）。
- 摄像头硬件能力不代表 Agent 已启用视觉 Tool。Tool、Skill 及设备类型发布配置为后续迭代。
- D1 不创建真实设备、语音服务、Agent Runtime、知识库或付费商业能力。
- 角色编码和设备类型编码创建后不可修改；删除被设备类型使用的角色返回 HTTP 409。

## API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET/POST | /api/characters | 角色列表、创建 |
| GET/PATCH/DELETE | /api/characters/:id | 角色详情、编辑、删除 |
| GET/POST | /api/device-types | 设备类型列表、创建 |
| GET/PATCH/DELETE | /api/device-types/:id | 设备类型详情、编辑、删除 |

所有 API 复用登录会话或 API Token 身份、统一响应、RBAC 和操作审计。D1 列表暂不分页，仅用于小规模产品配置原型。

## A3 / A4 演示

1. 进入后台「角色资源」，创建编码 `shinchan` 的演示角色。
2. 进入「设备类型」，创建 A3，关联该角色，不启用摄像头。
3. 创建 A4，关联同一角色，启用摄像头。
4. 更新角色人格，A3 和 A4 均仍引用同一个角色 ID。
5. 在 A3/A4 删除前直接删除角色应得到 HTTP 409。

## 数据库与授权升级

首次初始化的空库按现有流程自动建表、安装 root 授权目录。

**已初始化的数据库必须显式升级授权目录**：部署新代码且数据库可用后，执行：

```bash
go run . upgrade-access-authorization --config configs/config.toml
```

升级会写入新增的角色资源 / 设备类型 API、菜单、权限和 root grants。普通非 root 角色需要由管理员另外授权；正常服务启动不会静默升级授权。不要在备份缺失时直接操作生产库。

## D1 验收（CI 必须全部通过）

- Go: `go test -race -count=1 ./...`、`go vet`、静态分析、构建
- MySQL 8.4: 真实数据库的角色及设备类型 CRUD、唯一索引、外键、数据持久化
- 旧库升级: 先初始化已有数据库，再模拟缺失的 Root 授权，执行同一个 `UpgradeAccessAuthorization` 入口，验证恢复及二次升级幂等
- Chromium E2E: 真实 Umi + Go + MySQL，管理员登录、角色管理、A3/A4、共享角色更新、引用删除保护及清理
- 前端: `npm run lint`、`npm run test`、`npm run build`

普通本地单元测试不需要数据库，集成测试仅在设置以下变量时启用：

```bash
# 仅使用一次性测试库，切勿指向开发或生产数据库。
export PERSONAOS_TEST_MYSQL_DSN='ci_user:ci-test-only@tcp(127.0.0.1:3306)/personaos_ci?parseTime=true&charset=utf8mb4'
go test -count=1 -v ./internal/modules/catalog/adapters/mysql -run '^TestCatalogMySQLCompleteLifecycle
# 旧库升级回归测试要求数据库已经完成系统初始化。
go test -count=1 -v ./internal/boot -run '^TestUpgradeExistingDatabaseRestoresD1Catalog
```

完整浏览器验收见 `tests/e2e/catalog_ui.py`；GitHub Actions 负责启动独立的 MySQL、Go API 和 Umi，再使用 Playwright/Chromium 执行。
**以上命令的测试用例会写入、修改测试库记录，请勿对真实业务数据库运行。**

D1 不包括 Tool、Skill、Agent Runtime 或真实设备注册，这些属于后续迭代。


# 旧库升级回归测试要求数据库已经完成系统初始化。
go test -count=1 -v ./internal/boot -run '^TestUpgradeExistingDatabaseRestoresD1Catalog

```

完整浏览器验收见 `tests/e2e/catalog_ui.py`；GitHub Actions 负责启动独立的 MySQL、Go API 和 Umi，再使用 Playwright/Chromium 执行。
**以上命令的测试用例会写入、修改测试库记录，请勿对真实业务数据库运行。**

D1 不包括 Tool、Skill、Agent Runtime 或真实设备注册，这些属于后续迭代。


```

完整浏览器验收见 `tests/e2e/catalog_ui.py`；GitHub Actions 负责启动独立的 MySQL、Go API 和 Umi，再使用 Playwright/Chromium 执行。
**以上命令的测试用例会写入、修改测试库记录，请勿对真实业务数据库运行。**

D1 不包括 Tool、Skill、Agent Runtime 或真实设备注册，这些属于后续迭代。


# 旧库升级回归测试要求数据库已经完成系统初始化。
go test -count=1 -v ./internal/boot -run '^TestUpgradeExistingDatabaseRestoresD1Catalog

```

完整浏览器验收见 `tests/e2e/catalog_ui.py`；GitHub Actions 负责启动独立的 MySQL、Go API 和 Umi，再使用 Playwright/Chromium 执行。
**以上命令的测试用例会写入、修改测试库记录，请勿对真实业务数据库运行。**

D1 不包括 Tool、Skill、Agent Runtime 或真实设备注册，这些属于后续迭代。
