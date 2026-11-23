# D2-2：独立能力资源目录

此增量为 D2-3 设备类型草稿配置提供可引用的资源目录，不改变 D1 的设备类型表，也不启用任何设备能力。

## 资源

| Kind | Reference 语义 | 备注 |
| --- | --- | --- |
| `voice` | 已配置 TTS Provider 下的音色标识 | Provider 必填 |
| `tool` | 平台已注册的工具处理器标识 | 资源关联不表示获准执行 |
| `skill` | 已安装的 Skill 标识 | 不是任意可执行代码 |
| `knowledge_base` | 检索系统中的知识库索引标识 | 实际内容管理和检索在后续接入 |

每条资源具有独立 ID、类型、不可变编码、名称、说明、可选 Provider、Reference 与启停状态。数据库 `capability_resources` 对 `(kind, code)` 设置联合唯一索引，允许不同种类复用编码。

`Reference` 只存不含凭据的内部查找键。API Key、Token、账号密码及可执行代码不得写入该字段；关联时还需检查资源的有效性与权限。后台实际接入 Provider 时另行实现凭据管理。

## 当前实现

- `domain.NewResource` 验证资源类型、编码及文本边界。
- `mysql.NewResourceStore` 通过已有 GORM 连接维护资源表，提供按类型列表、详情、创建、修改和删除。
- `NewResourceStore` 当前**尚未注册到 boot composition root**，因此运行中的后台不会自动建表或暴露新 API。D2-3 将统一完成启动装配与受管 API 目录。
- `Delete` 只适用于当前没有设备类型绑定表的阶段。D2-3 必须增加关联外键/引用保护后，才能开放后台删除能力。
- 此阶段**没有**把资源关联到设备类型，也**没有**执行、发布或加载这些资源。

## 验收

```bash
go test ./internal/modules/catalog/domain
make test
make lint

# 仅针对一次性 MySQL 测试库运行
export PERSONAOS_TEST_MYSQL_DSN='ci_user:ci-test-only@tcp(127.0.0.1:3306)/personaos_ci?parseTime=true&charset=utf8mb4'
go test -count=1 -v ./internal/modules/catalog/adapters/mysql -run '^TestResourceStoreMySQLLifecycle$'
```

真实 MySQL 测试会验证各类型编码隔离、同类编码唯一、重开存储后读取、启停更新、删除和取消上下文。GitHub CI 的 MySQL Smoke 会运行此用例。

## 下一步

D2-3 应以 `device_type_id` 为聚合配置入口，并在同一数据库事务中验证资源存在性和可用状态，保存 Voice/Tool/Skill/Knowledge Base 关联。随后补齐 usecase、HTTP、RBAC、审计、Web UI 与设备型号差异化验收。此阶段仍属于草稿，不做发布、设备实例或 Agent Runtime。
