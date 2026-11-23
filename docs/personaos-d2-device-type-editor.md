# D2-3c：设备类型能力草稿编辑器

后台设备类型页面新增「能力草稿」操作，按照设备类型分别关联 Voice、Tool、Skill 和 Knowledge Base。

- 一个设备类型最多选一个 Voice，可选多个 Tool、Skill、知识库。
- **角色是共享资源，能力草稿属于设备类型**：A3/A4 即使关联同一角色，也不会互相覆盖能力草稿。
- 设备硬件能力（摄像头、麦克风、扬声器）与获准使用的工具、Skills 分离；不会因支持摄像头自动授予视觉工具。
- 草稿读取：`GET /api/device-types/:id/capability-draft`。
- 资源目录：并行按四种资源类型读取 `GET /api/capability-resources?kind=...`。
- 保存：`PUT /api/device-types/:id/capability-draft` 传完整资源 ID 集合，后端在事务中检查资源存在、类型、启用状态，失败回滚。
- 页面显示已停用资源的标识；如草稿包含已停用资源，须删除相应引用才能保存。
- 页面访问受既有设备类型读取权限约束；能力草稿读取还需能力资源读取权限，编辑需设备类型更新权限。真正访问由现有 RBAC 受管 API grants 校验；失败时禁止覆盖草稿。
- 关闭编辑器会卸载编辑状态，切换设备时重新读取，不共享临时表单状态。

仅支持**未发布草稿**。这一步不涉及设备实例继承、上线发布、运行时 Tool 执行授权或知识库内容管理。

## 验收

1. A3、A4 关联同一个角色，分别设置 Voice/Tool/Skill/知识库，刷新与重新打开后配置各自保持独立。
2. Voice 未选择时 PUT 发送 `voice_id: 0`，四个数组字段始终被完整传输。
3. 没有能力资源读取 API grants 时显示加载失败且禁止保存；仅有查看权限时隐藏保存能力。
4. 已停用或不存在资源由服务端拒绝，原有草稿保持不变。
5. Go race / lint、Web lint / 测试 / 构建、真实 MySQL 与浏览器 CI 全部通过后合并。

已初始化的部署环境如尚未执行 PR #11/#12 的授权升级，需要先备份数据库，运行：

```bash
go run . upgrade-access-authorization --config configs/config.toml
```
