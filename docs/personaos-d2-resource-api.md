# D2-3c：能力资源管理 API

本增量开放独立能力资源的后台管理接口，不改变角色、设备类型以及已经存在的草稿能力关联的语义。

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| GET | `/api/capability-resources?kind=voice` | `capability_resource:read` |
| GET | `/api/capability-resources/:id` | `capability_resource:read` |
| POST | `/api/capability-resources` | `capability_resource:create` |
| PATCH | `/api/capability-resources/:id` | `capability_resource:update` |
| DELETE | `/api/capability-resources/:id` | `capability_resource:delete` |

`kind` 只能是 `voice`、`tool`、`skill`、`knowledge_base`，列表接口必须明确传入类型。

创建请求示例：

```json
{
  "kind": "voice",
  "code": "narrator_a",
  "name": "旁白音色",
  "provider": "aliyun",
  "reference": "voice_narrator_a",
  "description": "默认播报",
  "enabled": true
}
```

修改只更新名称、说明、提供方、引用键和启停状态；资源 ID、Kind、Code 保持稳定。删除被设备类型能力草稿引用的资源受 MySQL 外键约束保护，返回冲突，而不是解除绑定。

写操作由现有操作日志组件记录成功或失败。请求须经过现有受管 API 的会话/API Token 认证与角色 API 授权；权限 Token 在角色管理中供展示及授权目录使用，实际 HTTP 防护依赖受管 API Grant。

**部署注意：** 对已初始化的数据库，先备份，再执行：

```bash
go run . upgrade-access-authorization --config configs/config.toml
```

这会同步新的部署自有 API、权限目录与超级管理员授权；普通角色需要按最小权限显式授予新 API 的访问权。

本 PR 仅开放 API，不新增后台菜单页面。下一个 UI 增量将提供资源管理列表和设备类型草稿编辑器。资源引用不是可执行代码，也不是已经发布到设备的能力；平台不会使用草稿自动授权工具运行。
