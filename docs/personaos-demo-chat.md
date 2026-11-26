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
PERSONAOS_LLM_EXTRA_FIELDS='{"enable_thinking":false}'
```

`PERSONAOS_LLM_EXTRA_FIELDS` 是可选的单行 JSON 对象；例如支持 `enable_thinking` 的模型可设置 `{"enable_thinking":false}`，其他厂商按其 API 文档填写。也可在第三方服务配置 → OpenAI 兼容模型服务 → 额外请求参数中保存，下一次请求生效。留空不发送额外参数，不允许覆盖模型、消息、工具、流式协议等运行时管理的字段，不要放入凭据。

`PERSONAOS_LLM_BASE_URL` 是可选值，留空使用模型适配器的默认服务地址；使用其他服务时明确配置，例如云厂商兼容地址。密钥和模型名不完整时，后台仍可启动，但真实对话会返回未配置错误，不生成假回答。

## 演示操作

1. 在后台新增任意设备类型、关联一个 Character。
2. 保存设备类型的能力草稿并点击「应用配置」。
3. 应用成功后点击「开始对话测试」，直接进入对应设备类型的对话；也可从「AI 对话演示」选择型号。顶部显示当前发布版本与 LLM、ASR、当前音色 TTS、RAG 初始化状态（不代表云端服务已实测连通）。点击「新建会话」清除本轮工作台历史，长期记忆开关保持不变。
4. 修改角色人格并重新应用配置后，新对话会使用更新的人格。只改草稿而不应用，不会影响运行中的角色配置。

## Tool 执行

Tool Registry 根据已生效的设备类型配置构建 Eino 原生 `[]tool.BaseTool`，由 Eino ToolsNode 负责实际调用、结果回传和循环。后台创建 Tool 资源时选择内置 Tool 或 HTTP Tool，再从服务端实际注册的执行器或批准的端点标识中选择。目前可用 `get_current_time`，返回服务器当前 UTC 时间（RFC3339）。将资源关联到设备类型并应用配置后，Eino 可自行调用该工具并根据返回值回答。未启用、未关联、未应用或没有注册执行器的资源不会暴露给模型。资源 reference 不作为脚本、URL 或命令执行。

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

服务端环境变量 `PERSONAOS_HTTP_TOOL_ENDPOINTS='{"weather":"https://weather.example.com/query"}'` 注册端点，后台 reference 填写 weather。GET 参数编码到 query，POST 以 JSON 请求体发送。第一版支持 object 下的 string、number、integer、boolean 参数；不支持任意 JSON Schema 扩展。工具描述和配置随设备类型快照发布；端点由部署方批准，不允许模型指定地址，不跟随跳转。仅配置批准的只读查询 API；不在资源或地址中放入凭据。HTTP 调用有 3 秒超时、8 KiB 参数和 16 KiB 响应上限。新增已批准端点的工具不需要修改 Agent 代码。

### 联网查询

在能力资源中创建 provider 为 `builtin` 的 Tool，并在设备类型能力草稿中关联及应用：

| reference | 参数 | 能力 | 服务端配置 |
| --- | --- | --- | --- |
| get_weather | city（城市名） | 当前天气、未来 3 天预报 | `PERSONAOS_OPEN_METEO_DEMO=1` |
| web_search | query（搜索问题） | 最多 3 条网页标题、链接和摘要 | `DEEPSEEK_API_KEY`（DeepSeek API Key） |

Open-Meteo 公共接口仅适用于符合其非商业使用条款的演示，商用或内部商业用途请先获得适当授权、接入授权端点；默认不启用。DeepSeek 使用独立 Search API，不获取网页全文。天气来源为 Open-Meteo（https://open-meteo.com/），地名数据来自 GeoNames。未配置的工具不暴露给模型；网络失败返回可读的 Tool 错误，不编造结果。查询参数和结果均有大小及时间限制，模型不能指定服务地址或认证信息。搜索内容是外部数据，不是指令。

## Skills

创建 Skill 资源，从已安装技能列表选择。目前提供 `storytelling`。关联设备类型并应用后，Eino 的 Skill middleware 会向模型提供该技能的名称、描述；模型需要时调用原生 `skill` 工具加载完整说明。技能以当前 Agent 的上下文执行，不创建子 Agent，不更换模型。未关联或未启用的技能既不可见也不可加载。

技能说明随服务端代码发布，后台 reference 不接受任意文件路径或脚本。

## 知识库

使用 Redis 8（Search）、Eino Redis Indexer/Retriever 和 Eino Ark Embedding。设置 `PERSONAOS_EMBEDDING_API_KEY`，启用 Redis 配置；向量模型固定为 `doubao-embedding-vision-251215`（1024 维）。

后台「能力资源 → 知识库」支持直接选择 UTF-8 TXT 或 Markdown 文档（最多 64 KiB），保存时同步向量化并生成不可变版本。原文存入 MySQL `knowledge_sources`，索引成功后才创建或更新资源引用；已有发布快照不受影响。也可使用 CLI 离线入库：

```bash
go run . --config configs/config.toml knowledge-index character-lore-v1 ./character-lore.txt
```

UTF-8 文本按空行及最多 600 字符分段，单文件最多 1 MiB。后台创建 Knowledge Base 资源，provider 为 `redis`，reference 为已经索引的版本 `character-lore-v1`，关联设备类型并应用。更新内容须先索引新的版本，再修改并发布资源引用。原始文档应保留用于重建索引。

在「能力资源 → 知识库」中点击「索引与检索」，查看当前版本在 Redis Search 中实际可检索的分片数量，并直接输入问题测试 Eino Retriever 的前三个分片。此操作不调用 LLM，不会修改资源版本或已应用配置。状态检查不产生 Embedding 调用；检索测试会调用 Embedding。已有数据库需要执行 `make upgrade-access` 授权新增的知识库检查与测试 API。

Agent 按需调用 `search_knowledge`，以当前快照中的已授权版本过滤 Redis 向量检索，返回最相近的三个片段。未绑定知识库时不注册该工具。无可用配置或版本不存在时显式报错，不回退旧检索器。文字和语音共用检索逻辑。

## 待接入

真实设备动作和视觉感知尚未接入。

## 会话与实时观测

网页 Trajectory 请求始终发送 `session_id`（UUID），即使关闭长期记忆。服务端按管理员、当前角色、设备类型隔离会话，维护最近 20 条成功交互消息；不再信任网页提供的 history。同一会话一次仅执行一轮，失败不写入历史。

`GET /api/demo/sessions/:id/events?after=0` 是独立、最长等待 20 秒的事件订阅接口。事件包含会话内递增的 sequence、turn、时间及 user/trace/reply 数据。订阅者使用最后收到的 sequence 作为下一次 after；gap 为 true 表示较早记录已淘汰。取消订阅不会取消发送端。网页发送端仍使用当前 NDJSON 播放音频和显示自身的调试过程。发送一条消息后，点击「打开观测窗口」，同账号、同当前角色的新标签页独立订阅该 Session，同时查看对话和 Trajectory。保持观测窗口开启，再从发送端发送消息即可验证实时同步；观测窗口可断开、重新连接并从上次事件序号补拉，不会取消发送端。文字及语音回复通过 `reply_preview` 发送有界累积文本预览（最多每 50 ms 更新一次，首次立即发送），两端更新同一回复气泡；`turn_done` 中完整回复为准，预览不等于成功完成。观测端没有发送、录音、播放权限入口，不接收原始音频；仅浏览器本地测量的 ASR/TTS 指标仍只显示在发送端。

单进程会话最多 64 个，每会话保留最近 256 个事件，空闲 15 分钟按访问时清理，重启丢失；不保存原始音频。设备端后续采用 WebSocket，需独立设备身份认证及音频帧协议，尚未对外开放设备端点。新增受管订阅 API 部署到已有数据库后需执行 `make upgrade-access`。

`GET /api/demo/resource-options` 返回已注册 Tool、Skill、SDK 支持的语音厂商，不返回端点 URL 或凭据（需能力资源读取权限）。`GET /api/demo/status` 返回 Agent 依赖的初始化状态（需设备类型读取权限）。已有数据库升级后执行 `make upgrade-access` 以更新受管路由授权。
