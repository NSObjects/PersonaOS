# PersonaOS D2-3b：设备类型能力草稿 API

本增量将已有 D2-3a 草稿存储装配进默认应用，仍不对设备发布能力。

## 已接入

- Catalog 模块启动时按依赖顺序迁移 Character/Device Type、Capability Resource、Capability Draft 三组表。
- `GET /api/device-types/:id/capability-draft` 返回一个设备类型当前未发布的 Voice/Tool/Skill/Knowledge Base 关联；尚未设置则返回空选择。
- `PUT /api/device-types/:id/capability-draft` 原子替换整个关联配置，请求示例：

```json
{
  "voice_id": 0,
  "tool_ids": [],
  "skill_ids": [],
  "knowledge_base_ids": []
}
```

- 输入领域校验拒绝无效和重复资源 ID；MySQL 事务校验类型、启用状态与引用完整性，失败全部回滚。
- 读写路由登记在部署拥有的 Managed API Catalog：读取继承 `PermissionDeviceTypeRead`，写入继承 `PermissionDeviceTypeUpdate`。HTTP 由现有会话/API Token、RBAC 中间件保护，写入记入操作审计。
- 已初始化的数据库必须先备份，再执行显式授权目录升级：

```bash
go run . upgrade-access-authorization --config configs/config.toml
```

## 本阶段未做

独立能力资源的管理 HTTP/API、后台资源选择 UI、角色默认音色配置及真实 Knowledge Base 导入不属于当前 PR。运行时不加载草稿、不授权工具执行，设备实例也不继承草稿。

## 验收

新增 usecase、HTTP 单元测试；既有路由授权目录一致性、Go Race、静态分析、MySQL 集成测试及 Chromium E2E 由统一 CI 验证。
