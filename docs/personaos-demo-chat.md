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

## Tool 执行

Tool Registry 根据已生效的设备类型配置构建 Eino 原生 `[]tool.BaseTool`，由 Eino ToolsNode 负责实际调用、结果回传和循环。后台创建 Builtin Tool 资源，provider 为 `builtin`，`reference` 填写已注册的执行器标识。目前可用 `get_current_time`，返回服务器当前 UTC 时间（RFC3339）。将资源关联到设备类型并应用配置后，Eino 可自行调用该工具并根据返回值回答。未启用、未关联、未应用或没有注册执行器的资源不会暴露给模型。资源 reference 不作为脚本、URL 或命令执行。

同一 Agent 运行逻辑同时用于文字和流式语音回复。Tool 结果由 Eino 回传模型，不作为用户回复或 TTS 文本。工具循环最多四次模型调用。

### HTTP Tool

后台创建 Tool 资源：provider 为 `http`，code 为模型看到的工具名，description 为用途，reference 为服务端批准的端点标识。HTTP Tool 配置使用 JSON，例如：

```json
{
  "method": "GET",
  "input_schema": {
    "type": "object",
    "properties": {
      "city": {"type": "string", "description": "城市名称"}
    },
    "required": ["city"]
  }
}
```

服务端环境变量 `PERSONAOS_HTTP_TOOL_ENDPOINTS='{"weather":"https://weather.example.com/query"}'` 注册端点，后台 reference 填写 weather。GET 参数编码到 query，POST 以 JSON 请求体发送。第一版支持 object 下的 string、number、integer、boolean 参数；不支持任意 JSON Schema 扩展。工具描述和配置随设备类型快照发布；端点由部署方批准，不允许模型指定地址，不跟随跳转。仅配置批准的只读查询 API；不在资源或地址中放入凭据。HTTP 调用有 3 秒超时、8 KiB 参数和 16 KiB 响应上限。新增已批准端点的工具不需要修改 Agent 代码。MCP 尚待接入。

## Skills

创建 Skill 资源，reference 使用已安装技能名。目前提供 `storytelling`。关联设备类型并应用后，Eino 的 Skill middleware 会向模型提供该技能的名称、描述；模型需要时调用原生 `skill` 工具加载完整说明。技能以当前 Agent 的上下文执行，不创建子 Agent，不更换模型。未关联或未启用的技能既不可见也不可加载。

技能说明随服务端代码发布，后台 reference 不接受任意文件路径或脚本。

## 知识库

在服务端设置 `PERSONAOS_KNOWLEDGE_DIR`，在目录中放置 UTF-8 文本文件，例如 `character-lore-v1.txt`。用空行划分独立知识段落，每个文件最大 64 KiB。创建 Knowledge Base 资源，provider 填写 `local`，reference 填写不含扩展名的文件标识 `character-lore-v1`，关联设备类型并应用配置。

Agent 只获得已绑定的知识库检索工具 `search_knowledge`；Eino 调用检索器，按中英文关键词匹配取最多三个相关片段，返回来源和内容供模型回答。没有命中时返回空结果。禁止把文档当作指令；未绑定的文档不会进入检索范围。已绑定本地知识库缺失或非法时返回配置错误，不悄悄忽略。服务端只读取专用目录中的文件，不接受 URL、任意路径或文件上传 ID。知识内容变更时使用新的版本化 reference。

当前为小规模文本知识库，未使用 Embedding 或向量数据库。文字与语音使用同一检索链路。

## 待接入

真实设备动作和视觉感知尚未接入。
