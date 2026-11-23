# AI 角色文字对话 Demo

## 组成

- **网页：** `@ant-design/x` 的 `Bubble.List` 与 `Sender`，页面路径 `/agent-demo`。
- **Agent：** Eino `adk.NewChatModelAgent` + `adk.NewRunner`，模型通过 `eino-ext/components/model/openai` 使用 OpenAI 兼容协议。
- **配置：** 通过 `ActiveConfigUsecase.Get` 获取某个后台创建的设备类型已应用的快照。角色人格作为 Eino Agent 的 Instruction，用户输入和有限历史作为消息。
- **API：** `POST /api/demo/chat`，使用后台现有登录/CSRF/RBAC 保护，复用 `device_type:read` 权限。此 API 不是设备公开接入协议。

## 模型环境变量

在**服务端**设置以下变量（禁止把 API Key 送入网页代码）：

```bash
PERSONAOS_LLM_API_KEY=<server-side-token>
PERSONAOS_LLM_MODEL=<model-name>
PERSONAOS_LLM_BASE_URL=<openai-compatible-api-base-url>
```

`PERSONAOS_LLM_BASE_URL` 是可选值，留空使用模型适配器的默认服务地址；使用其他服务时明确配置，例如云厂商兼容地址。密钥和模型名不完整时，后台仍可启动，但真实对话会返回未配置错误，不生成假回答。

## 演示操作

1. 在后台新增任意设备类型、关联一个 Character。
2. 保存设备类型的能力草稿并点击「应用配置」。
3. 进入「AI 对话演示」，选择对应设备类型，进行多轮文字对话。
4. 修改角色人格并重新应用配置后，新对话会使用更新的人格。只改草稿而不应用，不会影响运行中的角色配置。

## 暂不实现

这次不接真实设备音频、不运行 Tool 或 Skill、不查询 RAG，不存储长期对话历史，也不开放未经设备身份验证的公网接口。后续接入时必须以已生效的设备类型和实际设备硬件能力共同确定运行授权。
