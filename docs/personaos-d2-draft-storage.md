# PersonaOS D2-3a：设备类型草稿能力持久化

此增量在 D2-1 的 `DeviceTypeCapabilities` 和 D2-2 的独立能力资源目录之上，新增数据库级关联能力；并非完整 D2-3 后台交互功能。

## 数据约束

- `device_type_capability_drafts` 保存设备类型与独立资源的多对多关联，外键分别引用 `device_types` 和 `capability_resources`。
- 同一设备类型可以选一个 Voice、任意数量 Tool/Skill/Knowledge Base，不允许重复引用。
- 两个设备类型可以引用同一角色与同一资源，也可以配置不同资源；配置隔离依赖 `device_type_id`。
- 保存采用 MySQL 事务并锁定设备类型行。先校验全部关联资源存在、类型正确且已启用，然后整体替换；任何一项失败全部回滚，不会部分清空旧配置。
- 删除仍被使用的资源会受到数据库外键保护并返回冲突；删除仍带草稿关联的设备类型也由外键保护。
- 物理摄像头能力与 `tool` 类型的视觉工具引用保持分离，关联本身不等于发布或运行时授权。

## 边界与装配

本 PR 的 `NewCapabilityDraftStore` 是显式构造函数，尚未装配到应用启动入口，故现有 D1 管理 API 不变。D2-3b 要加入 Usecase、受管 HTTP API、RBAC、审计、后台 UI 和相关授权升级验收。后续 D3 再实现发布与设备实例继承。

## 验收

使用仅供测试的一次性 MySQL 数据库：

```bash
export PERSONAOS_TEST_MYSQL_DSN='ci_user:ci-test-only@tcp(127.0.0.1:3306)/personaos_ci?parseTime=true&charset=utf8mb4'
go test -count=1 -v ./internal/modules/catalog/adapters/mysql -run '^TestCapabilityDraftMySQLLifecycle$'
```

测试覆盖 A3、A4 共用 Character 但独立配置、类型混用拒绝、错误输入事务回滚、旧草稿保留、清空草稿，以及对已关联资源的删除保护。
