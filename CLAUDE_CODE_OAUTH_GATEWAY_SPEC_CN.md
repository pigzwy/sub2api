# Claude Code OAuth 独立网关开发规格

> 文档状态：可直接交给 AI 编程代理作为实现任务书
> 参考代码基线：本仓库提交 `ac8442ef`
> 复核说明：2026-07-13 已在当前 HEAD `1feeb115` 再次核对。相对先前复核点，`gateway_handler.go`、`gateway_helper.go`、`routes/gateway.go` 有其他协议/并发相关改动，但本文引用的 Claude OAuth、`/v1/messages` 反代和模拟核心行为未改变；源码索引已按当前 HEAD 更新。
> 核心目标：实现 Claude OAuth 登录、Token 管理、Claude Code 请求反代，以及可选客户端模拟的独立网关。
> 可选二开目标：第 16 节定义一个与核心反代解耦的调用方控制面，包含托管 Client Key、权限、用量账本和本地限量。
> 仍非目标：终端用户注册登录、余额充值、订阅售卖、账单结算、IP 白名单、分组、账号池调度，以及 OpenAI/Gemini/Bedrock/Vertex 协议桥。

## 0. 上线前提与风险

本方案使用 Claude Code/Claude 平台当前的 OAuth 客户端标识、scope、beta 和请求指纹行为。这些内容不是一个承诺长期稳定的通用第三方网关协议。开始开发和部署前必须确认：

1. 使用者拥有或被明确授权使用对应 Claude 账号，不得将个人 OAuth 凭据用于未授权共享、转售或多租户服务。
2. 部署和使用方式符合 Anthropic 当前服务条款。若目标是长期、公开、通用的商业 API 网关，应优先使用 Anthropic 正式 API Key，而不是 Claude Code-scoped OAuth。
3. OAuth Client ID、scope、CLI/Stainless 版本、beta 列表、System 结构及 TLS 特征都可能变化；它们必须集中配置、带兼容版本，并有真实账号回归流程。
4. MVP 的支持边界是“单账号、单进程、单副本”。不要用共享文件启动多个实例。多副本必须先引入共享凭据存储、跨实例刷新锁和共享 OAuth SessionStore。
5. 不要把 `/readyz` 或定时监控实现成高频真实推理请求。需要验证账号推理能力时，使用显式、受管理密钥保护的诊断操作，并说明它可能消耗额度。

兼容性失败应可被安全诊断，但不能通过日志泄露 Token、完整 System Prompt 或消息正文。建议为所有易变兼容参数增加一个 `compatibility_profile_version`，写入日志和健康摘要，以便升级后回滚。

## 1. 实现指令

请实现一个独立、可运行、可测试的 Claude Code OAuth 网关。技术栈优先使用 Go 标准库；只有在确有收益时引入小型、成熟依赖。实现必须遵守本规格中的安全边界、请求处理顺序、接口契约和验收测试。

第一阶段只支持一个 Anthropic OAuth 账号和官方 Claude Code 客户端。第二阶段再增加“非 Claude Code 客户端模拟”功能，并且必须由配置显式开启。第三阶段可按第 16 节增加调用方授权、用量和本地限量。不要从原项目复制用户、余额、分组、订阅、计费结算或账号池代码。

## 2. 目标架构

```text
管理员浏览器
  -> /admin/oauth/start
  -> Claude OAuth PKCE 授权
  -> /admin/oauth/exchange
  -> 加密或 0600 权限持久化 OAuth 凭据

Claude Code
  -> POST /v1/messages
  -> 网关静态密钥认证
  -> 识别真实 Claude Code
  -> 获取或刷新 Claude OAuth access_token
  -> 保留 Claude Code Body，重建安全上游 Header
  -> POST https://api.anthropic.com/v1/messages?beta=true
  -> JSON 或 SSE 无缓冲返回

Claude Code
  -> POST /v1/messages/count_tokens
  -> 同一认证与 Token 流程
  -> POST https://api.anthropic.com/v1/messages/count_tokens?beta=true
```

必须区分三类凭据：

1. `ADMIN_API_KEY`：只访问管理监听地址和 OAuth/Client 管理接口。
2. `GATEWAY_API_KEY` 或托管 `Client Key`：Claude Code/下游调用方访问数据接口。
3. `access_token` / `refresh_token`：网关访问 Anthropic 的 OAuth 凭据。

客户端绝不能获得上游 OAuth Token。网关收到客户端 `Authorization` 或 `x-api-key` 后，必须重建上游请求，不能把客户端认证头直接透传。

## 3. 范围

### 3.1 MVP 必须实现

- 单 Anthropic OAuth 账号。
- OAuth 2.0 Authorization Code + PKCE 登录。
- OAuth 临时会话、`state` 校验和一次性消费。
- OAuth 凭据安全持久化和原子更新。
- Access Token 到期前自动刷新，并发刷新合并。
- `POST /v1/messages`。
- `POST /v1/messages/count_tokens`。
- `GET /v1/models`，返回静态模型清单或可配置清单。
- `GET /healthz` 和 `GET /readyz`。
- 静态入口 API Key 认证。
- 官方 Claude Code 请求识别。
- Anthropic JSON 错误格式。
- 非流式 JSON 反代。
- SSE 无缓冲反代和客户端取消传播。
- 401 时刷新 Token 并在响应尚未提交的前提下重试一次。
- 结构化、脱敏日志。
- 单元测试和 `httptest` 集成测试。

### 3.2 第二阶段可选实现

- 非 Claude Code 客户端模拟 Claude Code。
- 稳定客户端指纹和 `metadata.user_id`。
- System Prompt 三块结构、billing attribution block。
- cache-control 断点。
- 工具名改写及响应逆向还原。
- `sessionKey` Cookie 自动 OAuth。
- 出站 HTTP/SOCKS 代理。

### 3.3 第三阶段可选控制面

- 无终端用户系统的服务调用方 `Client` 管理。
- 每个 Client 多把可撤销、可过期的托管 Client Key。
- 路由 scope、模型权限和模拟权限。
- 请求级用量账本和按 Client 聚合查询。
- RPM、并发、单请求输出上限、TPM、日/月/累计 Token 限额。
- 并发安全的额度预占、最终结算和异常流处理。
- 管理审计、Key 轮换和本地限额调整。

### 3.4 明确禁止引入

- 终端用户注册、密码/OIDC 登录和面向用户的角色系统。
- 余额、充值、订阅、优惠券、发票、财务账单和自动扣费。
- 把本地 Token 统计宣称为 Anthropic 官方套餐余额或官方用量。
- IP ACL 和分组体系；如未来需要，应作为独立扩展，不混入 Client Key MVP。
- 多账号选择、粘性调度和账号 failover。
- OpenAI Responses/Chat Completions 转换。
- Gemini、Antigravity、Bedrock 和 Vertex。
- 为了“兼容”而完整复制原项目的 GatewayService。

## 4. 推荐目录

```text
cmd/gateway/main.go

internal/config/config.go
internal/httpserver/router.go
internal/httpserver/middleware.go

internal/oauth/constants.go
internal/oauth/pkce.go
internal/oauth/session_store.go
internal/oauth/client.go
internal/oauth/service.go

internal/token/model.go
internal/token/store.go
internal/token/provider.go

internal/claude/types.go
internal/claude/detector.go
internal/claude/headers.go
internal/claude/beta.go
internal/claude/proxy.go
internal/claude/sse.go

internal/mimic/body.go       # 第二阶段
internal/mimic/metadata.go   # 第二阶段
internal/mimic/tools.go      # 第二阶段

internal/control/client.go   # 第三阶段
internal/control/key.go
internal/control/policy.go
internal/control/admin.go
internal/usage/model.go
internal/usage/store.go
internal/usage/collector.go
internal/limits/limiter.go
internal/limits/reservation.go
internal/audit/store.go

migrations/                 # 第三阶段启用持久化时使用
```

核心接口建议：

```go
type CredentialStore interface {
	Load(ctx context.Context) (*Credentials, error)
	Save(ctx context.Context, value *Credentials) error
	Delete(ctx context.Context) error
}

type OAuthClient interface {
	ExchangeCode(ctx context.Context, in ExchangeCodeInput) (*TokenResponse, error)
	RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error)
}

type TokenProvider interface {
	AccessToken(ctx context.Context) (string, error)
	RefreshRejectedToken(ctx context.Context, rejectedToken string) (string, error)
}

type Transformer interface {
	Transform(req *http.Request, body []byte) (wireBody []byte, state *TransformState, err error)
	RestoreResponseChunk(state *TransformState, chunk []byte) []byte
}
```

MVP 的 `Transformer` 对真实 Claude Code 应基本为 no-op；只允许模型别名规范化、可配置的 OAuth 身份隔离（定点重写 `metadata.user_id`），以及与 beta 能力严格对应的字段清理。

## 5. 配置契约

建议支持环境变量或 YAML，两者只能有一个明确优先级。敏感值不得在启动日志中打印。

```yaml
server:
  listen: "127.0.0.1:8080"
  admin_listen: "127.0.0.1:8081"
  read_header_timeout: 10s
  request_body_timeout: 60s
  idle_timeout: 120s
  write_timeout: 0s
  downstream_write_idle_timeout: 30s
  upstream_stream_idle_timeout: 5m
  shutdown_timeout: 30s
  max_header_bytes: 1048576
  max_body_bytes: 33554432
  max_sse_event_bytes: 4194304

auth:
  mode: "static" # static | managed
  gateway_api_key_env: "GATEWAY_API_KEY"
  admin_api_key_env: "ADMIN_API_KEY"

# 第三阶段启用；auth.mode=static 时不连接此数据库，也不启动用量/限量 worker。
control_plane:
  enabled: false
  capability_level: "auth" # auth | realtime_limits | usage | token_quotas
  database_driver: "postgres"
  database_dsn_env: "CONTROL_DATABASE_DSN"
  key_pepper_current_env: "CLIENT_KEY_PEPPER_V1"
  key_pepper_previous_env: ""
  auth_cache_ttl: 15s
  auth_negative_cache_ttl: 2s
  policy_cache_ttl: 15s
  hard_limits_fail_closed: true
  rpm_algorithm: "token_bucket"
  reservation_lease_ttl: 10m
  reservation_heartbeat_interval: 1m
  orphan_grace_period: 15m
  orphan_policy: "charge_reserved"
  usage_detail_retention: 2160h # 90d
  usage_query_max_range: 2160h
  usage_query_max_page_size: 200

anthropic:
  messages_url: "https://api.anthropic.com/v1/messages?beta=true"
  count_tokens_url: "https://api.anthropic.com/v1/messages/count_tokens?beta=true"
  oauth_authorize_url: "https://claude.ai/oauth/authorize"
  oauth_token_url: "https://platform.claude.com/v1/oauth/token"
  oauth_redirect_uri: "https://platform.claude.com/oauth/code/callback"
  oauth_client_id: "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
  oauth_scope: "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
  token_refresh_skew: 3m
  oauth_http_timeout: 60s
  oauth_user_agent: "axios/1.13.6"
  max_response_header_bytes: 1048576
  # 以下别名是参考基线当时的兼容快照，不是 Anthropic 的永久模型清单。
  model_aliases:
    claude-sonnet-4-5: "claude-sonnet-4-5-20250929"
    claude-opus-4-5: "claude-opus-4-5-20251101"
    claude-haiku-4-5: "claude-haiku-4-5-20251001"
  # /v1/models 的展示数据；完整示例和响应契约见第 6.5 节。
  model_catalog: []

credentials:
  path: "./data/claude-oauth.json"
  instance_id_path: "./data/instance-id"
  encryption: "required"
  encryption_key_env: "CLAUDE_GATEWAY_CREDENTIAL_KEY"

compatibility:
  profile_version: "2026-07-reference"
  client_policy: "claude_code_only"
  detection_mode: "compatible"
  rewrite_oauth_metadata: true
  mimic_non_claude_clients: false
  tls_fingerprint_mode: "off"
  cli_version: "<集中配置并可更新>"
  stainless_package_version: "<集中配置并可更新>"
  # 以下值也是参考快照。实现应把整个 profile 当作一个不可变配置快照加载。
  beta_profile:
    oauth_required:
      - "oauth-2025-04-20"
    official_messages_default:
      - "claude-code-20250219"
      - "oauth-2025-04-20"
      - "interleaved-thinking-2025-05-14"
      - "fine-grained-tool-streaming-2025-05-14"
    official_messages_haiku_default:
      - "oauth-2025-04-20"
      - "interleaved-thinking-2025-05-14"
    official_count_tokens_default:
      - "claude-code-20250219"
      - "oauth-2025-04-20"
      - "interleaved-thinking-2025-05-14"
      - "token-counting-2024-11-01"
    mimic_messages_default:
      - "claude-code-20250219"
      - "oauth-2025-04-20"
      - "interleaved-thinking-2025-05-14"
      - "prompt-caching-scope-2026-01-05"
      - "effort-2025-11-24"
      - "context-management-2025-06-27"
      - "extended-cache-ttl-2025-04-11"
    mimic_messages_haiku:
      - "oauth-2025-04-20"
      - "interleaved-thinking-2025-05-14"
    drop: []
    max_tokens: 32
    max_header_bytes: 4096
```

约束：

- 默认监听 `127.0.0.1`。对外暴露由部署者显式配置反向代理和 TLS。
- 管理接口优先使用独立的 loopback 监听地址；如果共用监听地址，也必须使用独立路由组和 `ADMIN_API_KEY`。
- `http.Server.WriteTimeout` 必须为 `0`，否则持续时间较长的 SSE 会被服务器统一写超时截断。使用上游响应头超时和连接级空闲检测控制资源，而不是总写超时。
- 使用 `http.MaxBytesReader` 限制 Body，并在读取请求体期间设置可配置读 deadline，防止慢速 Body 长时间占用连接；进入上游/SSE 阶段后清除该读 deadline。
- `downstream_write_idle_timeout` 是“每次下游写入”的空闲上限，不是整个响应总时长。可用 `http.NewResponseController(w).SetWriteDeadline` 在每次写入前刷新；不支持 deadline 的 Writer 应记录一次诊断并继续。
- `upstream_stream_idle_timeout` 是上游 SSE 连续没有任何字节的上限，每次读取到数据后重置；它不是总响应时长。设为 `0` 可关闭，但生产环境应结合 Anthropic keepalive 实测选择足够宽松的值。
- `credentials.path` 的文件权限必须为 `0600`。
- `instance_id` 首次启动时安全随机生成并持久化，用于稳定身份映射；不得每次重启变化。
- OAuth Client ID、Claude CLI 版本、Stainless 版本和整组 beta profile 都属于易变兼容参数，集中定义并允许配置覆盖；不能只更新其中一个值而留下半新半旧的指纹组合。
- 模型目录和模型别名同样属于易变数据。示例映射仅记录参考基线的行为，不代表 Anthropic 当前或未来的完整支持清单，必须允许删除/更新；未知模型默认透传。
- 上述 OAuth Client ID 是当前参考实现使用的公开客户端标识，不是客户端密钥；实现时仍应集中配置，以便上游变更后替换。
- 示例 `oauth_scope` 为参考项目的兼容快照，不等于最小权限：本网关运行时真正需要的是推理及 Claude Code session 相关权限；`org:create_api_key`、profile、MCP、file upload 是否仍为当前授权流程必需，必须通过真实账号回归确认。首版可为兼容使用参考值，但不得调用创建 API Key/MCP/file API；确认可删的 scope 后应收窄，并把变更纳入 profile 版本。不要在未回归时盲目删 scope 导致登录不可用。
- 不允许客户端传入任意上游 URL，避免 SSRF。
- 生产模式只允许预设的 HTTPS 上游 host。Mock URL 覆盖只能通过明确的测试模式开启。
- `GATEWAY_API_KEY` 和 `ADMIN_API_KEY` 必须非空且不能相同，否则启动失败。
- `auth.mode=static` 时要求 `GATEWAY_API_KEY`；`auth.mode=managed` 时禁止配置可绕过策略的 `GATEWAY_API_KEY`，并要求 control database 和当前 Client Key pepper。两种模式不能自动猜测或静默互相降级。
- `control_plane.enabled=true` 必须与 `auth.mode=managed` 一致。硬限量启用时 `hard_limits_fail_closed` 必须为 true；若运维明确改为 fail open，API/UI 不得把它标记为硬限制。
- `capability_level` 是累计能力声明：`auth`=Phase A，`realtime_limits`=A+B，`usage`=A+B+C，`token_quotas`=A-D。启动时必须验证相应 migration、依赖和 worker 已就绪；不能仅靠配置字符串宣称能力。未达到对应级别时，管理 API 对 `rpm/concurrent/max_tokens/TPM/周期额度` 等尚未支持的非 NULL 字段返回 422 `unsupported_feature`，不能保存后静默忽略。降级 capability 前也必须先清除或迁移依赖该能力的策略。
- Control DB DSN 和 Key pepper 只能从环境/Secret 读取，不进入配置 dump。数据库迁移必须由显式命令执行，网关进程默认不在生产启动时自动跑破坏性 migration。
- `*_env` 的值是环境变量名称，配置加载器必须从环境读取对应 Secret；不得把环境变量值回填到配置 dump 或日志。
- `credentials.encryption=required` 时，加密密钥缺失必须启动失败。只有本机开发可显式设置 `disabled` 并使用 `0600` 明文文件；禁止自动降级，也不要把加密密钥写在凭据文件旁边制造“伪加密”。
- 所有 duration、字节上限、并发上限和 beta profile 都要在启动时校验范围；未知配置键建议直接报错，避免拼写错误悄悄回退默认值。启动日志只打印配置来源、非敏感摘要和 `profile_version`，不打印 Secret 或完整凭据路径内容。

## 6. HTTP 接口

### 6.1 `POST /admin/oauth/start`

认证：`Authorization: Bearer <ADMIN_API_KEY>`。

请求体可为空：

```json
{}
```

响应：

```json
{
  "auth_url": "https://claude.ai/oauth/authorize?...",
  "session_id": "随机不可预测值",
  "expires_in": 1800
}
```

处理：

1. 使用 `crypto/rand` 生成 32 字节 `state`。
2. 生成 32 字节 `code_verifier`，使用无填充 base64url。
3. `code_challenge = BASE64URL(SHA256(code_verifier))`。
4. 生成不可预测 `session_id`。
5. 保存 `{state, code_verifier, created_at, status:"pending"}`，TTL 30 分钟。
6. 返回授权地址和 `session_id`。

OAuth 会话可在单实例内存保存。若部署多实例，必须使用共享存储或保证管理端粘性会话。

授权 URL 必须使用 `net/url` 结构化构造。当前参考参数为：

```text
https://claude.ai/oauth/authorize
  ?code=true
  &client_id=<oauth_client_id>
  &response_type=code
  &redirect_uri=https%3A%2F%2Fplatform.claude.com%2Foauth%2Fcode%2Fcallback
  &scope=<url-encoded configured scope>
  &code_challenge=<base64url sha256>
  &code_challenge_method=S256
  &state=<base64url random state>
```

参数顺序不应成为校验逻辑；测试应解析后比较键值。`code=true`、Client ID、redirect URI 和 scope 都属于当前兼容 profile，不是自定义回调协议。网关不需要在公网开放 OAuth callback，管理员从 Anthropic 页面复制完整 `code#state` 或 callback URL 回本地 exchange 接口。

### 6.2 `POST /admin/oauth/exchange`

请求：

```json
{
  "session_id": "...",
  "authorization_result": "authorization_code#returned_state"
}
```

`authorization_result` 必须兼容以下输入，便于管理员直接粘贴 Claude 页面结果：

```text
authorization_code#state
https://platform.claude.com/oauth/code/callback?code=...&state=...
?code=...&state=...
code=...&state=...
```

解析器只能 URL-decode 一次，限制输入长度（例如 16 KiB），并拒绝重复或冲突的 `code`/`state` 参数。裸 `code` 没有返回 state 时必须拒绝，提示管理员重新授权并粘贴完整结果。不要用正则拼凑 URL；使用 `net/url` 和显式的 `code#state` 分支。

强制行为：

1. 查找未过期、状态为 `pending` 的 OAuth 会话。
2. 按上述规则将 `authorization_result` 规范化为授权码和返回 state。
3. 使用常量时间比较校验返回 state 与保存 state。
4. state 缺失或不一致时拒绝，不得请求 Token 端点。
5. 原子地将会话从 `pending` 改为 `exchanging`；并发请求只有一个能成功 claim。
6. 使用保存的 `code_verifier` 换 Token。
7. 无论成功或失败都消费并删除该会话；失败后重新开始 OAuth，避免授权码/session 重放。
8. 成功时原子保存新凭据。
9. 响应只返回账号摘要和过期时间，不返回 Token。

> 参考项目的 Service 将 state 传给 Token 端点，但没有在 Service 层显式比较返回 state。独立实现必须补上本地 state 校验，不能照搬这一缺口。

Token 请求：

```http
POST https://platform.claude.com/v1/oauth/token
Content-Type: application/json
Accept: application/json, text/plain, */*
User-Agent: axios/1.13.6
```

```json
{
  "code": "<authorization_code>",
  "grant_type": "authorization_code",
  "client_id": "<oauth_client_id>",
  "redirect_uri": "https://platform.claude.com/oauth/code/callback",
  "code_verifier": "<saved_verifier>",
  "state": "<validated_state>"
}
```

Token 响应必须校验：`access_token` 非空、`token_type` 合法、`expires_in` 为合理正数。缺失或畸形响应不得覆盖现有凭据。`organization`/`account` 信息可能缺失，应允许为空；刷新响应缺失这些元数据时保留旧值。

OAuth Token HTTP Client 与推理 HTTP Client 分离。当前参考实现的 Token 请求 UA 为 `axios/1.13.6`，应作为可配置兼容参数。Token exchange/refresh 是短请求，可以设置总超时（例如 60 秒）；推理 SSE Client 不设置总超时。

Token、refresh 和推理 POST 一律禁止自动跟随重定向，或最多只允许经过严格校验的同 scheme/同 host 重定向。最安全的首版是 `CheckRedirect = http.ErrUseLastResponse`，收到 3xx 作为上游错误处理，避免 Authorization、refresh token 或请求正文被带到其他 host。

管理端最小交互闭环：

```text
1. 调用 /admin/oauth/start
2. 显示“打开授权链接”命令或按钮
3. 明确提示：完整复制 Claude 页面显示的 code#state（不要删除 # 后内容）
4. 将原始文本提交给 /admin/oauth/exchange
5. 成功后只展示账号摘要、scope 和 expires_at
```

若不开发管理网页，必须提供等价 CLI 子命令，例如 `gateway oauth login`，而不是要求用户手工拼 HTTP JSON。

#### 管理生命周期补充

建议同时提供两个只在管理监听地址开放的接口：

```http
GET  /admin/oauth/status
POST /admin/oauth/logout
```

`GET /admin/oauth/status` 使用 `ADMIN_API_KEY`，只返回低敏状态，不触发刷新：

```json
{
  "logged_in": true,
  "status": "ready",
  "expires_at": "2026-07-12T15:04:05Z",
  "scope": "user:inference user:sessions:claude_code",
  "token_version": 7,
  "compatibility_profile_version": "2026-07-reference"
}
```

不得返回 access/refresh token、邮箱、账号 UUID、加密密文或 Token 端点原始错误。`status` 使用与 `/readyz` 相同的内部状态枚举，但它是受保护的管理详情；健康端点只给更少的信息。

`POST /admin/oauth/logout` 请求体必须含显式确认值，例如 `{"confirm":"LOGOUT"}`。处理时先阻止新数据请求，再在凭据锁内清除内存、待持久化状态和 durable 凭据，最后进入 `login_required`；失败时保持 fail closed 并返回通用错误。它只代表“从本网关移除本地凭据”，不能声称已在 Anthropic 撤销 Token，除非未来接入并验证官方 revocation endpoint。不要宣称在 SSD/日志型文件系统上能够可靠安全擦除旧扇区。

重新授权不能先登出：`/admin/oauth/start` 只创建临时 session，不影响当前凭据；只有新 code exchange 成功且新凭据 durable Save 完成后，才原子替换旧凭据。Save 失败继续保留旧 durable 凭据且不宣告切换成功。限制 pending OAuth session 数量；单管理员 MVP 可在每次 start 时废弃此前未使用 session，避免粘贴错配。

### 6.3 `POST /v1/messages`

认证支持 Claude Code 常用配置：

```http
Authorization: Bearer <GATEWAY_API_KEY>
```

也可兼容：

```http
x-api-key: <GATEWAY_API_KEY>
```

如果两个头同时存在且值冲突，返回 401。使用常量时间比较密钥。

认证解析必须拒绝歧义：`Authorization` 或 `x-api-key` 任一 Header 出现多行/多值、Bearer 含逗号或额外字段、空值与非空值混合时都返回 401；不要依赖 `Header.Get` 静默选择第一个。只接受一个规范的 `Bearer <token>`，token 长度设置合理上限。两个认证头同时存在时，只有各自都唯一、格式合法且值相同才允许通过。

请求体最低校验：

- `Content-Encoding` 必须为空或 `identity`；MVP 不接受 gzip/brotli 压缩请求体，其他值返回 415。
- `Content-Type` 若存在，必须能解析为 `application/json`；可接受 charset 参数。
- 必须是合法 JSON object。
- `model` 必须是非空字符串。
- `messages` 必须存在且为数组。
- `stream` 如果存在必须是布尔值。
- Body 超限返回 413。

JSON 校验、Claude Code 检测、Body 定点改写和上游转发必须基于同一组原始字节与一致的解析语义。Go 的 `encoding/json` 默认接受重复键；实现应使用 token 级扫描拒绝顶层关键字段重复（至少 `model`、`messages`、`stream`、`system`、`tools`、`tool_choice`、`metadata`、`thinking`、`context_management`、`output_config`），并拒绝 `metadata.user_id` 重复。必须拒绝首个 JSON object 后还有第二个值或非空尾随数据。不能用一个解析器做检测、另一个取值规则不同的工具做改写，然后把含歧义的原始 Body 发给上游。

响应保持 Anthropic Messages 协议，不做 OpenAI 转换。

用于本地检测和 mock 上游联调的代表性请求（不是用于伪造真实生产流量的固定模板）：

```bash
curl -N http://127.0.0.1:8080/v1/messages \
  -H 'Authorization: Bearer <GATEWAY_API_KEY>' \
  -H 'content-type: application/json' \
  -H 'anthropic-version: 2023-06-01' \
  -H 'anthropic-beta: claude-code-20250219,oauth-2025-04-20' \
  -H 'x-app: cli' \
  -H 'user-agent: claude-cli/2.1.161 (external, cli)' \
  --data-binary @- <<'JSON'
{
  "model": "claude-sonnet-4-5",
  "max_tokens": 512,
  "stream": true,
  "system": [
    {
      "type": "text",
      "text": "x-anthropic-billing-header: cc_version=2.1.161.abc; cc_entrypoint=cli;"
    },
    {
      "type": "text",
      "text": "You are Claude Code, Anthropic's official CLI for Claude."
    }
  ],
  "metadata": {
    "user_id": "user_0000000000000000000000000000000000000000000000000000000000000000_account_11111111-1111-1111-1111-111111111111_session_22222222-2222-4222-8222-222222222222"
  },
  "messages": [
    {
      "role": "user",
      "content": [{"type": "text", "text": "Reply with ok"}]
    }
  ]
}
JSON
```

真正使用官方 Claude Code 时，不应手写这些指纹 Header，而是配置：

```bash
export ANTHROPIC_BASE_URL="http://127.0.0.1:8080"
export ANTHROPIC_AUTH_TOKEN="<GATEWAY_API_KEY>"
claude
```

### 6.4 `POST /v1/messages/count_tokens`

与 `/v1/messages` 使用同一入口认证、Claude Code 识别、Token 获取和上游 Header 策略。

上游请求前删除这些无关字段：

```text
temperature, top_p, top_k, stream, stop_sequences, stop
```

必须包含 `token-counting-2024-11-01` beta。上游不支持时原样返回 Anthropic 404，让 Claude Code 自行 fallback。

### 6.5 `GET /v1/models`

认证：与数据端使用相同的 `GATEWAY_API_KEY`，不能匿名暴露。返回 Anthropic Models API 风格的本地静态列表；此路由不调用 Anthropic，也不触发 Token 刷新。模型清单必须配置化，不要把未来模型名写进协议分支。

无分页的最小精确响应示例：

```json
{
  "data": [
    {
      "id": "claude-sonnet-4-5-20250929",
      "type": "model",
      "display_name": "Claude Sonnet 4.5",
      "created_at": "2025-09-29T00:00:00Z",
      "capabilities": null,
      "max_input_tokens": null,
      "max_tokens": null
    },
    {
      "id": "claude-haiku-4-5-20251001",
      "type": "model",
      "display_name": "Claude Haiku 4.5",
      "created_at": "2025-10-01T00:00:00Z",
      "capabilities": null,
      "max_input_tokens": null,
      "max_tokens": null
    }
  ],
  "has_more": false,
  "first_id": "claude-sonnet-4-5-20250929",
  "last_id": "claude-haiku-4-5-20251001"
}
```

空列表时 `data=[]`、`has_more=false`、`first_id=null`、`last_id=null`。不要返回原项目的 `{"object":"list",...}`，那是其兼容层的自定义/OpenAI 风格 envelope，不是 Anthropic Models API 的分页 envelope。按当前 Anthropic ModelInfo schema，`capabilities`、`max_input_tokens`、`max_tokens` 应存在但允许为 `null`；没有可靠配置时返回 `null`，不能编造能力值。将来 schema 变化时随 compatibility profile 更新契约测试。

MVP 可以明确拒绝 `before_id`、`after_id` 或 `limit`（400），也可以实现真正分页；不能忽略分页参数却错误声称 `has_more=true`。列表顺序由配置确定，并据实际返回页计算 `first_id`/`last_id`。

`/v1/models` 是展示清单，不是请求白名单。默认对 `/v1/messages` 的未知非空模型名透明传给 Anthropic，让上游决定是否支持；否则 Claude 发布新模型后网关会无故阻断。只有管理员显式配置独立的 `allowed_models` 时才在本地限制。第 5 节的三个 4.5 别名和本节两个模型项都只是参考基线示例，不是稳定或完整清单。

### 6.6 健康检查

- `/healthz`：进程存活即 200。
- `/readyz`：配置有效、凭据可解密/解析，且存在当前可用或具备 refresh token 的 OAuth 凭据时 200；未登录、凭据损坏或已确定无法刷新时 503。刷新成功但新凭据尚未持久化时属于 `credential_store_degraded`，必须返回 503，即使当前进程仍可暂时使用内存中的新 access token。不要为了每次 readiness 检查主动刷新或调用推理端点。
- 健康响应不得包含邮箱、账号 UUID 或凭据。
- 可返回低基数状态码，例如 `ready`、`login_required`、`credential_store_error`、`credential_store_degraded`、`refresh_failed`；不要返回上游原始错误 Body。

## 7. OAuth 和 Token 管理

### 7.1 凭据模型

```go
type Credentials struct {
	SchemaVersion int       `json:"schema_version"`
	TokenVersion  uint64    `json:"token_version"`
	AccessToken   string    `json:"access_token"`
	RefreshToken  string    `json:"refresh_token,omitempty"`
	TokenType     string    `json:"token_type"`
	Scope         string    `json:"scope,omitempty"`
	ExpiresAt     time.Time `json:"expires_at"`
	OrgUUID       string    `json:"org_uuid,omitempty"`
	AccountUUID   string    `json:"account_uuid,omitempty"`
	Email         string    `json:"email,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}
```

### 7.2 持久化

最低要求：

1. 目录权限尽量为 `0700`，文件权限 `0600`。
2. 写同目录临时文件、`fsync` 文件、`rename` 原子替换，并在支持的平台 `fsync` 父目录，保证崩溃后的目录项持久性。
3. 写入过程中不得产生 world-readable 文件。
4. 日志和错误响应不得出现 Token。
5. 如果使用配置加密密钥，密钥只能来自环境或外部 Secret，不得和密文同文件保存。
6. 启动时验证目录可写、文件属主/权限和 JSON schema；不满足要求时 fail closed，不带凭据启动 `/readyz=503`。
7. 加密建议使用 AES-256-GCM 或 XChaCha20-Poly1305，每次随机 nonce，并用 schema version/instance ID 作 AAD；密文需要明确格式版本，支持以后迁移。
8. 正常路径采用 write-through：刷新成功后的新凭据先持久化，再发布到内存。唯一例外是上游刷新已成功而持久化失败；此时旧 refresh token 可能已被轮换失效，必须把新凭据发布为“仅内存、待持久化”的权威版本，进入 `credential_store_degraded`，禁止悄悄回退旧凭据。
9. 获取独占进程文件锁，禁止两个网关进程同时读写同一凭据文件。锁失败时启动失败；文件锁不是多副本协调方案。
10. `instance-id` 也使用 `0600` 和原子写。备份/恢复必须同时保留凭据密文、instance ID 和外部加密密钥，否则身份映射或解密会失败。
11. 启动时拒绝凭据文件、锁文件或 instance ID 是符号链接，验证它们是当前服务用户拥有的普通文件；创建时使用限制性 umask/显式 mode，避免 TOCTOU 和链接覆盖其他路径。

持久化失败状态机必须明确实现：

```text
durable
  -- refresh 成功 + Save 失败 --> credential_store_degraded

credential_store_degraded:
  authoritative = 内存中的新 Credentials（包含轮换后的 refresh token）
  /healthz       = 200
  /readyz        = 503, status=credential_store_degraded
  新请求         = 可配置；建议 fail closed 返回 503，避免进程崩溃后丢失唯一新 refresh token
  当前请求       = 默认返回 503；不能因 Save 失败重新调用 refresh
  后台动作       = 只重试 Save(authoritative)，指数退避+jitter+上限；绝不再次调用 Token 端点
  Save 成功      = durable，清除告警并恢复 ready
  进程退出       = 仍 degraded 时输出高优先级脱敏告警；磁盘旧凭据不得覆盖内存新版本
```

若产品明确选择“degraded 时继续用内存 access token”以提高可用性，也必须作为显式配置并在文档中声明重启会丢失新凭据；`/readyz` 仍为 503，且不得启动第二次刷新。无论采取哪种流量策略，TokenProvider 都必须先记住新 `TokenVersion` 和新 refresh token，再返回持久化错误，否则下一请求会拿旧 refresh token 重刷。

首次 OAuth code exchange 与 refresh 不同：如果新凭据从未成功持久化，管理接口必须返回失败且 `/readyz=503`；可以在进程内短暂保留它并仅重试落盘，但不能向管理员宣告登录完成。已有 durable 凭据也不得被半写文件覆盖。

### 7.3 自动刷新

TokenProvider 算法：

```go
func (p *Provider) AccessToken(ctx context.Context) (string, error) {
	if p.pendingPersist() != nil {
		// 不允许从磁盘旧值覆盖内存中的轮换后凭据，也不允许再次刷新。
		return "", ErrCredentialStoreDegraded
	}

	cred := p.current()
	if cred != nil && time.Until(cred.ExpiresAt) > p.refreshSkew {
		return cred.AccessToken, nil
	}

	value, err, _ := p.refreshGroup.Do("claude-oauth", func() (any, error) {
		cred := p.reloadOrCurrent()
		if cred == nil {
			return "", ErrLoginRequired
		}
		if time.Until(cred.ExpiresAt) > p.refreshSkew {
			return cred.AccessToken, nil
		}
		if cred.RefreshToken == "" {
			return "", ErrLoginRequired
		}
		refreshCtx, cancelRefresh := context.WithTimeout(p.serviceContext, p.refreshTimeout)
		next, err := p.oauth.RefreshToken(refreshCtx, cred.RefreshToken)
		cancelRefresh()
		if err != nil {
			return "", p.handleRefreshError(cred, err)
		}
		mergeRotatedTokenFields(next, cred)
		next.TokenVersion = cred.TokenVersion + 1

		// HTTP 刷新和持久化使用两个独立的有限时长 Context，避免刷新刚好
		// 耗尽 deadline 后把本可成功的 Save 误判为落盘故障。
		persistCtx, cancelPersist := context.WithTimeout(p.serviceContext, p.persistTimeout)
		saveErr := p.store.Save(persistCtx, next)
		cancelPersist()
		if saveErr != nil {
			// Refresh 已经成功，next 现在是唯一权威版本。发布 degraded
			// 状态只为避免 refresh-token rotation 后退回旧值；默认不放行流量。
			p.setPendingPersist(next)
			p.schedulePersistRetry(next)
			return "", ErrCredentialStoreDegraded
		}
		p.setCurrent(next)
		return next.AccessToken, nil
	})
	if err != nil {
		return "", err
	}
	return value.(string), nil
}
```

这是安全语义伪代码，不要求照抄锁结构。`setPendingPersist`、读取当前凭据、保存成功后的状态切换必须在同一互斥/状态机保护下；后台 Save 重试也要核对 `TokenVersion`，旧任务不得覆盖更新版本。不能在 pending persist 时调用 `reloadOrCurrent()` 后选择磁盘旧版本。

刷新请求：

```json
{
  "grant_type": "refresh_token",
  "refresh_token": "<secret>",
  "client_id": "<oauth_client_id>"
}
```

若刷新响应不返回新的 `refresh_token`，保留旧值；若返回则原子替换。刷新失败不得清空仍可诊断或重新授权的旧凭据。

刷新错误必须按 OAuth 错误码和 HTTP/网络类别处理，不能只按字符串模糊匹配：

| 类别 | 例子 | 行为 |
|---|---|---|
| 终态凭据错误 | `invalid_grant`、明确的 refresh token revoked/expired | 标记 `login_required`，`/readyz=503`，停止自动刷新，等待重新 OAuth |
| 配置错误 | `invalid_client`、固定 endpoint/client ID 配置错误 | 标记 `oauth_config_error`，`/readyz=503`，人工修复配置；不要循环请求 |
| 暂态错误 | DNS/TLS/timeout、HTTP 408/429/5xx | 保留凭据，进入带 jitter 的指数退避；尊重有界 `Retry-After` |
| 畸形 2xx | 非 JSON、缺少 access token、非法 expires_in | 不覆盖旧凭据，按上游协议错误告警并退避 |

主动提前刷新遇到暂态错误时，如果旧 access token 按本地时钟仍未过期，可以在配置的安全余量内继续使用旧 token，并将健康摘要标为 `refresh_degraded`；一旦过期则返回 503。被上游 401 明确拒绝的 token 绝不能因刷新失败而回退复用。必须记录 `next_refresh_attempt_at` 或等价冷却状态，避免每个新请求都打 Token 端点；日志只记录错误类别、HTTP 状态和脱敏 request ID，不记录原始 OAuth 错误 Body。

TokenProvider 还必须提供“被上游拒绝的 Token”语义，而不是无条件 ForceRefresh：

```go
func (p *Provider) RefreshRejectedToken(ctx context.Context, rejected string) (string, error) {
	return p.singleflight(func() (string, error) {
		current := p.current()
		if current.AccessToken != rejected {
			// 另一个请求已经刷新，直接复用新 Token，不能再刷新一次。
			return current.AccessToken, nil
		}
		return p.refreshAndPersist(p.newRefreshContext(), current)
	})
}
```

这条检查对于 refresh token rotation 是强制要求。仅使用 singleflight 仍可能在第一轮完成后由排队请求开启第二轮刷新。

实际刷新使用独立、有限时长的服务 Context（例如 30 秒），不能直接复用第一个触发者的请求 Context。等待刷新结果的各客户端仍可按各自 Context 提前退出，但其中一个客户端断开不能取消已经被其他请求共享的刷新操作。可使用 `singleflight.DoChan` 加每个 waiter 自己的 `select` 实现。

同理，OAuth code exchange 一旦已经收到成功 Token 响应，凭据持久化应在短时、独立 Context 中完成，不能因为管理端浏览器断开而丢失已交换出的凭据。

### 7.4 可选 `sessionKey` Cookie 登录

此功能不是 MVP，默认关闭。`sessionKey` 相当于 Claude Web 登录凭据，必须按最高敏感级别处理：只在请求内存中短暂存在，不写日志、不持久化、不回显。

当前参考流程：

1. 获取组织：

   ```http
   GET https://claude.ai/api/organizations
   Cookie: sessionKey=<secret>
   ```

2. 如果存在多个组织，参考实现优先 `raven_type=team`，否则取第一个。独立网关更稳妥的做法是返回组织列表让管理员明确选择，避免静默选错。
3. 生成新的 PKCE verifier/challenge 和 state。
4. 获取授权码：

   ```http
   POST https://claude.ai/v1/oauth/<organization_uuid>/authorize
   Cookie: sessionKey=<secret>
   Origin: https://claude.ai
   Referer: https://claude.ai/new
   Content-Type: application/json
   ```

   ```json
   {
     "response_type": "code",
     "client_id": "9d1c250a-e61b-44d9-88ed-5944d1962f5e",
     "organization_uuid": "<selected_uuid>",
     "redirect_uri": "https://platform.claude.com/oauth/code/callback",
     "scope": "user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload",
     "state": "<state>",
     "code_challenge": "<challenge>",
     "code_challenge_method": "S256"
   }
   ```

5. 从响应 `redirect_uri` 查询参数提取 `code` 和 `state`，在本地严格校验 state。
6. 使用第 6.2 节相同 Token 端点换取凭据。

如果实现此管理接口，应增加独立开关 `enable_session_key_login=false`，只允许管理监听地址访问，并对请求速率设低上限。

## 8. Claude Code 识别

识别只用于选择“透传”还是“模拟”，不是安全认证。入口安全边界始终是 `GATEWAY_API_KEY`。

### 8.1 MVP 建议判定

采用两级检测，不要把易变 System Prompt 当成安全凭据：

```text
基础命中（默认 compatible 模式）：
  User-Agent 匹配 ^claude-cli/\d+\.\d+\.\d+
  AND metadata.user_id 能按 Claude Code legacy 或 JSON 格式解析

增强信号（记录诊断，不作为默认硬拒绝条件）：
  system 存在已知 Claude Code 身份块或 billing attribution block
  X-App、anthropic-version、anthropic-beta 非空
```

`strict` 检测模式可以要求增强信号，但默认关闭，因为 Claude Code 的 Agent SDK、compact、安全监视和未来版本可能改变 System 形态。检测无法阻止持有网关密钥的客户端伪造 Header，所以不能把 strict 模式宣传为安全边界。

合法 `metadata.user_id`：

```text
旧格式：user_<64hex>_account_<uuid或空>_session_<uuid>
新格式：{"device_id":"...","account_uuid":"...","session_id":"..."}
```

特殊请求：

- `/messages/count_tokens`：通常没有完整 System Prompt，UA 命中即可按 Claude Code 辅助请求处理。
- `max_tokens=1` 且 Haiku 的连通性探测：UA 命中即可绕过 System/metadata 检查。
- 非标准子请求如果 UA 命中但 metadata 缺失，应记录低敏诊断字段并按 `client_policy` 处理；不要打印 Body。

MVP 若只服务官方 Claude Code，可以在检测失败时返回 403：

```json
{
  "type": "error",
  "error": {
    "type": "permission_error",
    "message": "This gateway accepts Claude Code requests only"
  }
}
```

`client_policy=claude_code_only` 时，不满足上述 compatible 规则且不是已知辅助请求才返回 403。第二阶段启用模拟时，检测失败进入 Transformer，不再返回 403。

## 9. `/v1/messages` 上游请求处理顺序

处理顺序属于协议不变量，不得随意调整：

```text
1. 限制并读取客户端 Body
2. 校验 JSON 和最低字段
3. 识别真实 Claude Code
4. 获取有效 access_token
5. 真实 Claude Code：尽量保持 Body 原始字节；若启用身份隔离，只定点改写 `metadata.user_id`
6. 可选模拟：完成所有 Body 改写
7. 计算最终 anthropic-beta
8. 按最终 beta 清理不被启用的 Body 能力字段
9. 创建新的上游 Request
10. 写入 OAuth Authorization
11. 写入白名单 Header 或模拟 Header
12. 最后覆盖 anthropic-beta
13. 发出请求
14. 收到 2xx 后选择 JSON 或 SSE 返回
```

真实 Claude Code 路径禁止：

- 全量反序列化再序列化 Body。
- 替换其 System Prompt。
- 重排 `messages`、`tools` 或 content blocks。
- 删除合法 thinking/signature。
- 除下述 OAuth 身份隔离外，随意重写 `metadata.user_id`。

原因是这些字段影响 Claude Code 行为、prompt cache 和签名兼容性。

### 9.1 OAuth 身份隔离

当前参考项目会对所有 Anthropic OAuth 请求（包括官方 Claude Code）定点重写 `metadata.user_id`，但不会因此进入完整 System/messages 模拟。目的有两个：

1. 上游看到的 `account_uuid` 与实际 OAuth 账号一致。
2. 不直接透传客户端设备 ID 和原始 session ID；同时保持同一会话稳定。

建议独立网关将它做成默认开启、可关闭的 `rewrite_oauth_metadata`：

```text
original = ParseMetadataUserID(body.metadata.user_id)
deviceID = 网关持久化的 64hex client/device ID
sessionID = UUID-format(SHA256(gatewayInstanceID + "::" + original.sessionID)[0:16])
accountUUID = OAuth token 响应中的 account_uuid
format = 根据客户端 claude-cli 版本保留 legacy 或 JSON 格式
```

必须使用 JSON 定点修改，只改 `metadata.user_id` 的字符串值，不重新序列化整个 Body。该功能无法获得合法原始 metadata 时应 fail-open 保持原值，而不是破坏请求。

如果入站存在 `X-Claude-Code-Session-Id`，重写 metadata 后必须将此 Header 同步为映射后的 `sessionID`；Header 与 Body 不一致会形成明显指纹。若关闭 metadata 重写，则两者都保持客户端原值。

## 10. 上游 Header 策略

不要使用通用 ReverseProxy 直接复制全部 Header。创建全新的 Header 集合。

### 10.1 必须覆盖

```http
authorization: Bearer <Claude OAuth access_token>
content-type: application/json
anthropic-version: <validated client value or configured fallback>
anthropic-beta: <computed value>
```

对于已识别的官方 Claude Code，合法的入站 `anthropic-version` 应保留；只有缺失时才回退到当前配置默认值（参考实现为 `2023-06-01`）。该值必须配置化，不能散落硬编码。应限制为短 ASCII token，拒绝 CR/LF 和异常长度。

MVP 不复制客户端 `Accept-Encoding`，交给 Go Transport 默认协商和自动解压；返回解压后的内容时不得复制上游 `Content-Encoding` 和 `Content-Length`。如果显式设置了 `Accept-Encoding`，Go 不再保证自动解压，必须自行实现并测试。模拟模式若要求精确压缩指纹，应作为单独、可关闭的兼容特性。

### 10.2 官方 Claude Code 可透传白名单

```text
accept
accept-language
user-agent
x-app
x-stainless-retry-count
x-stainless-timeout
x-stainless-lang
x-stainless-package-version
x-stainless-os
x-stainless-arch
x-stainless-runtime
x-stainless-runtime-version
x-stainless-helper-method
anthropic-dangerous-direct-browser-access
x-claude-code-session-id
x-client-request-id
```

注意：`accept-encoding` 不在白名单中，原因见上一节。

不得透传：

```text
authorization
x-api-key
cookie
host
content-length
connection 及其他 hop-by-hop headers
forwarded / x-forwarded-*（除非有明确用途）
```

`anthropic-beta` 不直接复制，必须拆分、去空格、去重、补必需 token、过滤禁止 token后重新写入。

`x-client-request-id` 若缺失则生成新的 UUID；若入站存在则只接受合法 UUID。`X-Claude-Code-Session-Id` 同理只接受合法 UUID，并遵守第 9.1 节的同步规则。所有 Header 值都应设置长度上限。

### 10.3 Beta 规则

Beta 不是越多越好。官方 Claude Code 已经发送 `anthropic-beta` 时，以客户端声明的能力为准，只补网关使用 OAuth 凭据所必需的 token；不要无条件把模拟模式的所有实验能力追加进去。否则 Header 可能声明 Body 并未按对应版本构造的能力，也会改变真实客户端行为。

当前参考决策矩阵如下，所有 token 都来自第 5 节的版本化 profile：

| 路由/客户端 | 入站 `anthropic-beta` | 最终值 |
|---|---|---|
| `/v1/messages` + 官方 Claude Code | 非空 | 保留合法入站 token，只补 `oauth_required`；当前参考仅强制 `oauth-2025-04-20` |
| `/v1/messages` + 官方 Claude Code | 缺失 | 使用模型相关的 `official_messages_default`；当前参考 Haiku 使用 `official_messages_haiku_default` |
| `count_tokens` + 官方 Claude Code | 非空 | 与上一行相同，再强制 `token-counting-2024-11-01` |
| `count_tokens` + 官方 Claude Code | 缺失 | 使用 `official_count_tokens_default`，其中必须含 token-counting |
| `/v1/messages` + 模拟客户端 | 任意 | 不信任/不混入客户端 beta；非 Haiku 使用完整 `mimic_messages_default`，Haiku 使用 `mimic_messages_haiku` |
| `count_tokens` + 模拟客户端 | 任意 | 不信任/不混入客户端 beta；使用完整 mimic profile，再补 token-counting。为简单和稳定，count_tokens 不按 Haiku 缩减 |

“当前参考”不等于永久规则。参考基线以模型名大小写不敏感地包含 `haiku` 作为 Haiku 分支；独立实现应把模型分类与 profile 一起版本化。未知模型走非 Haiku 默认，不在本地拒绝。

模拟模式的非 Haiku 完整 beta 参考快照为：

```text
claude-code-20250219
oauth-2025-04-20
interleaved-thinking-2025-05-14
prompt-caching-scope-2026-01-05
effort-2025-11-24
context-management-2025-06-27
extended-cache-ttl-2025-04-11
```

不得把此列表视为永久协议。必须集中配置并为合并/去重/过滤编写测试。参考项目的 `count_tokens` 模拟分支会混入客户端 beta；独立网关建议有意不复制这个不一致行为，使两个模拟路由都只使用服务端 profile。

解析和合并规则：

1. 按逗号拆分，逐项 trim；token 必须是长度受限的可打印 ASCII，不得包含空白、控制字符或再次包含逗号。
2. 使用完整 token 精确比较，不能用 `strings.Contains` 判断是否已存在。例如 `not-oauth-2025-04-20-x` 不能满足 OAuth 必需项。
3. 保持服务端 required token 的配置顺序，再按客户端原顺序追加其余 token；精确去重。
4. 应用集中配置的 deny/drop 集合；若某 required token 同时被 drop，启动时配置校验失败，不能静默生成缺少 OAuth 必需项的请求。
5. 对最终 Header 设置总长度和 token 数量上限，超限返回 400，不截断。
6. Body 能力与最终 beta 必须对称。当前参考实现只明确处理 `context_management`：最终值缺少 `context-management-2025-06-27` 时，定点删除顶层 `context_management`；不要声称已经支持所有 beta 字段的自动清理。
7. Body 清理必须发生在任何依赖最终 Body 的 billing/CCH 签名之前；清理失败应拒绝请求，不能发送 Header/Body 不一致的请求。

## 11. 反代与重试

### 11.1 上游 HTTP Client

- `ResponseHeaderTimeout` 应有限，例如 60 秒。
- 不要为整个 SSE Body 设置短总超时。
- 合理设置 Dial、TLS handshake、idle connection 超时。
- 使用请求 Context，让客户端断开能取消上游。
- 限制空闲连接和每主机连接数，避免资源泄漏。
- OAuth、messages 和 count_tokens 都必须从配置好的固定 URL 创建新请求，忽略客户端入站 query string。客户端不能覆盖 `beta=true`、host、path 或 scheme；使用 `url.URL` 结构化校验/构造，不用字符串拼接任意输入。
- 默认将 `Transport.Proxy=nil`，不要无意继承宿主机的 `HTTP_PROXY`/`HTTPS_PROXY`。若第二阶段支持出站代理，必须显式配置、校验 scheme，并在日志/错误中隐藏代理凭据。
- 出站请求默认使用 Go 标准 TLS/HTTP Transport。TLS 指纹模拟不是 MVP 必需功能；只有真实账号回归明确证明标准 Transport 被拒绝时，才引入可关闭、带 profile version 的 uTLS 模式。不要把 uTLS 与 HTTP/2 兼容性问题混入首版。
- `http.Client.Timeout` 应为 `0`，由 Dial/TLS/ResponseHeaderTimeout 和请求 Context 分阶段控制长流。
- 服务端 `http.Server.WriteTimeout=0`；如需防慢客户端耗尽资源，应通过反向代理的流式友好策略、连接并发上限和显式空闲检测解决。

### 11.2 重试矩阵

单账号 MVP 采用保守策略：

| 场景 | 行为 |
|---|---|
| 建连或读取响应头网络错误 | 默认不重试；上游可能已处理请求 |
| 上游 401 | 调用 `RefreshRejectedToken(rejectedToken)`，最多重试 1 次 |
| 上游 403 | 原样返回，不盲目重试 |
| 上游 429/529 | 原样返回并保留 `retry-after` |
| 其他 5xx | 默认原样返回，不重试 |
| 已向客户端写出任何响应 | 绝不重试 |
| SSE 已开始 | 绝不重试或拼接新流 |

每次重试必须重新创建 `http.Request` 和 Body Reader。

生成请求不是天然幂等。即使客户端尚未收到响应，建连后断开也可能代表上游已经接受并计费。因此 MVP 不应对网络错误或 5xx 自动重发。未来若增加请求重试，必须有上游明确支持的幂等键或经过验证的“请求未发送”信号。

### 11.3 401 单次刷新伪代码

```go
rejectedToken := token
resp, err := send(rejectedToken)
if err == nil && resp.StatusCode == http.StatusUnauthorized && !responseCommitted {
	drainAndClose(resp.Body, smallLimit)
	newToken, refreshErr := provider.RefreshRejectedToken(ctx, rejectedToken)
	if refreshErr == nil {
		resp, err = send(newToken) // 仅一次
	}
}
```

必须防止多个并发 401 导致刷新风暴，具体版本检查见第 7.3 节。

401 重试只适用于上游明确返回 HTTP 401、响应尚未提交且刷新确实成功的场景。刷新失败时向客户端返回通用 Anthropic `authentication_error`/502，并把网关标记为 not ready；不得把上游响应中的 Token 诊断细节直接回显。

## 12. 响应处理

### 12.1 非流式

1. 在提交下游 Header 前取得上游状态。
2. 对成功非流式响应和错误响应分别设置大小上限，例如 64 MiB 和 1 MiB，避免无限内存使用。
3. 复制安全响应头，例如 `content-type`、`x-request-id`、`request-id` 和限流头。
4. 原样写出状态码和 JSON Body。
5. 不向客户端暴露内部代理地址或网络错误详情。

若请求的顶层 `model` 经 `model_aliases` 改写，则成功响应必须对客户端保持一致：非流式只在顶层响应 `model` 精确等于映射后 ID 时还原为客户端原值；SSE 只在 `message_start.message.model` 精确匹配时还原。不要全局字符串替换，也不要改写错误消息、文本内容或未知位置。若不实现响应还原，MVP 应默认关闭模型别名映射。

建议响应 Header 白名单：

```text
content-type
x-request-id
request-id
retry-after
anthropic-ratelimit-*
```

不得复制 `Connection`、`Transfer-Encoding`、`Keep-Alive`、`Proxy-*`、上游 `Content-Length`，以及在内容被解压/改写后失效的 `Content-Encoding`/ETag。

错误映射：

| 来源 | 下游状态/类型 |
|---|---|
| 入口密钥错误 | `401 authentication_error` |
| 请求 JSON/字段错误 | `400 invalid_request_error` |
| Body 超限 | `413 invalid_request_error` |
| 未登录/需重新登录 | `503 authentication_error` |
| 凭据刷新成功但持久化失败 | `503 api_error`，健康状态 `credential_store_degraded` |
| Token 端点暂时不可用且没有仍有效的旧 token | `503 api_error` |
| 上游 400/403/429/529 | 保留状态和合法 Anthropic 错误体 |
| 上游 401 且刷新重试仍失败 | `502 authentication_error` |
| DNS/TLS/网络错误 | `502 api_error` |
| 上游响应头超时 | `504 api_error` |
| 网关内部错误 | `500 api_error` |

只有上游 Body 是大小受限、合法的 Anthropic 错误 envelope 时才原样传递；否则生成通用错误，并只在服务端保存脱敏摘要。

即使客户端请求了 `stream=true`，也必须先检查上游状态码和 `Content-Type`。只有 2xx 且 Content-Type 为 `text/event-stream` 时进入 SSE；2xx 但返回 JSON 时按有界非流式响应处理，非 2xx 则按错误路径处理，不能把 HTML/JSON 当 SSE 无限转发。

### 12.2 SSE

响应头：

```http
Content-Type: text/event-stream
Cache-Control: no-cache
X-Accel-Buffering: no
```

通常不应手工写 hop-by-hop 的 `Connection` 头；Go HTTP Server 会管理连接，HTTP/2 也禁止该头。只有明确受控的 HTTP/1 代理环境确实需要时才设置 `Connection: keep-alive`。

MVP 没有工具名/模型名逆向转换时，应按块读取、写入并立即 `Flush`，或给 `io.CopyBuffer` 包一层每次 `Write` 后 Flush 的 Writer。裸 `io.CopyBuffer` 不保证每次写入都立即 Flush。只要启用了工具名或模型别名响应还原，就必须按完整 SSE event（空行边界）增量解析，保留 `event:`、多行 `data:`、注释和原始顺序；不能假设一次 `Read` 等于一个事件。不要使用 `bufio.Scanner` 的默认 64 KiB 行上限；解析器必须设置 `max_sse_event_bytes`，超限时结束流并记录脱敏错误。

SSE 路由必须禁用 gzip/brotli 响应压缩中间件和通用响应缓存。反向代理也必须关闭缓冲；例如 Nginx 使用 `proxy_buffering off`。若部署在会强制缓冲或限制长连接的平台，必须单独做真实流式验收。

伪代码：

```go
flusher, ok := w.(http.Flusher)
if !ok {
	writeAnthropicError(w, 500, "api_error", "streaming not supported")
	return
}

buf := make([]byte, 32*1024)
for {
	n, readErr := resp.Body.Read(buf)
	if n > 0 {
		if _, writeErr := w.Write(buf[:n]); writeErr != nil {
			return // request context 取消上游
		}
		flusher.Flush()
	}
	if readErr != nil {
		if readErr != io.EOF && !responseCommitted {
			writeAnthropicError(...)
		}
		return
	}
}
```

若流已经开始后上游异常，不能再写普通 JSON。可以结束连接；若实现流内错误，必须发送标准 Anthropic SSE：

```text
event: error
data: {"type":"error","error":{"type":"api_error","message":"Upstream stream failed"}}

```

纯字节透传模式不要自行插入 keepalive 数据，以免改变 Anthropic 事件序列。只有在确认 Claude Code 可接受的前提下，才能启用协议合法且版本受控的 keepalive；首版默认关闭。

## 13. 可选模拟模式

只有 `compatibility.mimic_non_claude_clients=true` 时启用。模拟用于让非 Claude Code 客户端使用 Claude Code-scoped OAuth；官方 Claude Code 不进入此路径。

### 13.1 模拟 Header

集中定义并可更新，例如当前参考值：

```http
User-Agent: claude-cli/<configured-version> (external, cli)
X-Stainless-Lang: js
X-Stainless-Package-Version: <configured-version>
X-Stainless-OS: Linux
X-Stainless-Arch: arm64
X-Stainless-Runtime: node
X-Stainless-Runtime-Version: v24.3.0
X-Stainless-Retry-Count: 0
X-Stainless-Timeout: 600
X-App: cli
Anthropic-Dangerous-Direct-Browser-Access: true
Accept: application/json
x-client-request-id: <每请求新 UUID>
```

流式请求添加 `x-stainless-helper-method: stream`。

不要混合客户端指纹和模拟指纹：进入模拟路径后，除明确允许字段外，不透传客户端 Header。

### 13.2 模拟 Body 顺序

```text
1. 保存原始 system 文本
2. system 替换为 Claude Code 三块结构
3. 原 system 作为 user/assistant 消息对插入 messages 前部
4. 规范化 model
5. 确保 tools 至少为 []
6. 注入稳定 metadata.user_id
7. 可选重建 message cache-control
8. 可选改写工具名
9. 在 tools 最后一项添加 cache-control
10. 强制 cache-control 总数不超过 4
11. 保存本请求工具名映射用于响应恢复
```

三块 System 结构：

```json
[
  {
    "type": "text",
    "text": "x-anthropic-billing-header: cc_version=<version>.<fp>; cc_entrypoint=cli;"
  },
  {
    "type": "text",
    "text": "You are Claude Code, Anthropic's official CLI for Claude."
  },
  {
    "type": "text",
    "text": "<可配置的中性扩展提示>",
    "cache_control": {"type": "ephemeral", "ttl": "5m"}
  }
]
```

原始 System 迁移：

```json
[
  {
    "role": "user",
    "content": [{"type":"text","text":"[System Instructions]\n<original>"}]
  },
  {
    "role": "assistant",
    "content": [{"type":"text","text":"Understood. I will follow these instructions."}]
  }
]
```

### 13.3 Billing fingerprint

当前参考实现算法：

```text
firstText = 第一条 role=user 消息的第一段 text
chars = firstText[4] + firstText[7] + firstText[20]，缺失补 '0'
fp = HEX(SHA256("59cf53e54c78" + chars + cliVersion))[0:3]
```

输出：

```text
x-anthropic-billing-header: cc_version=<cliVersion>.<fp>; cc_entrypoint=cli;
```

这属于当前兼容行为，不是公开稳定协议。必须单独封装和做 golden test。

### 13.4 `metadata.user_id`

生成 32 字节随机 `device_id`，编码为 64 位 hex并持久化。一次对话的 `session_id` 必须稳定，不能每个请求随机变化。

可采用：

```text
seed = gatewayInstanceID + clientID + 第一条用户消息
sessionID = UUID-format(SHA256(seed)[0:16])
```

新 CLI 格式：

```json
{"device_id":"<64hex>","account_uuid":"<uuid>","session_id":"<uuid>"}
```

旧 CLI 格式：

```text
user_<64hex>_account_<account_uuid>_session_<uuid>
```

### 13.5 工具名改写

这不是 MVP 必需功能。若实现，必须同步改写：

- `tools[*].name`
- `tool_choice.name`
- 历史 `messages[*].content[*]` 中 `type=tool_use` 的 `name`
- 非流式响应和每个 SSE chunk 中的工具名必须逆向还原

禁止改写 Anthropic server tools，例如 `web_search_*`、`computer_*`。

## 14. 安全要求

- 管理 OAuth 接口和数据接口使用不同静态密钥。
- 密钥比较使用 `subtle.ConstantTimeCompare`。
- 默认只监听 loopback。
- 管理接口应额外支持来源限制或仅绑定独立管理监听地址。
- 不记录 Authorization、Cookie、code、verifier、access token、refresh token。
- 错误日志中的 URL 不得含代理密码或敏感查询参数。
- 限制请求体、错误体和非流式响应体大小。
- 固定上游 host；配置 URL 时启动阶段校验 scheme/host。
- 禁止开放通用 URL proxy 参数。
- OAuth session 必须有 TTL、一次性使用、state 校验。
- 生产环境通过 TLS 暴露；不要明文公网传输网关密钥。
- 优雅关机时先标记 not-ready、停止接收新请求，再给正在进行的 SSE 到 `shutdown_timeout`；超时后取消服务根 Context 并关闭残留连接，不能无限等待长流。
- 对数据路由设置全局并发上限和单客户端并发上限，避免单账号网关被无限长 SSE 耗尽文件描述符；达到上限返回 429，不实现余额或计费。
- 管理接口设置严格速率限制、`Cache-Control: no-store`，并拒绝浏览器跨域访问；若提供网页管理端，还要增加 CSRF 防护和严格 CSP。
- 配置热重载不能半更新 Header/版本组合。首版建议修改配置后重启；若实现热更，使用不可变配置快照原子切换。

## 15. 日志与指标

建议结构化字段：

```text
request_id
route
method
model
stream
is_claude_code
mimic_enabled
upstream_status
upstream_request_id
duration_ms
first_byte_ms
retry_count
token_refreshed
token_version
compatibility_profile_version
error_class
```

绝不记录：

```text
gateway_api_key
admin_api_key
authorization code
OAuth state/verifier
access_token
refresh_token
sessionKey Cookie
完整请求 Body（默认）
```

## 16. 可选二开：调用方授权、用量与限量控制面

本节是第三阶段的可直接开发规格。它不改变“单 Anthropic OAuth 账号”的上游模型，也不引入终端用户、余额或订阅系统。实现后形成下面的边界：

```text
Client / Client Key / Policy / Usage / Limits = 下游调用方控制面
Claude OAuth Credentials                  = 上游 Anthropic 凭据

两者只通过 request context 中的 client_id/key_id/policy_snapshot 关联，
Client Key 永远不能读取或管理 Claude OAuth Token。
```

### 16.1 功能范围与模式

支持两种互斥认证模式：

| 模式 | 用途 | 数据来源 |
|---|---|---|
| `static` | 单人/单调用方最小部署，保持现有 MVP | 环境变量 `GATEWAY_API_KEY` |
| `managed` | 多个受控调用方、独立权限和限量 | 数据库中的 `clients/client_keys/client_policies` |

生产环境只能选择一种模式，不能在 managed 模式下保留一个绕过所有策略的静态 Key。迁移时可提供有截止时间的 `migration_static_key`，但它必须映射到一个真实 Client/Policy、记录用量并能被撤销。`ADMIN_API_KEY` 不得用于 `/v1/*` 数据接口。

第三阶段建议至少交付：

1. Client CRUD、启停和备注。
2. Client Key 创建、列表、轮换、撤销和过期。
3. 路由 scope、模型 allow/deny、是否允许模拟模式。
4. RPM 和并发硬限制。
5. 单请求 `max_tokens` 限制。
6. 请求级 Token 用量账本。
7. 按分钟 TPM，以及日/月/累计 Token 限额。
8. 用量/限量状态查询和管理审计。

暂不做：余额、美元计价、套餐售卖、自动续期、用户自助门户、组织/租户层级、IP ACL、Webhook 和多账号分摊。

Client 创建后默认 `disabled`。管理员必须先写入完整 Policy、创建并安全交付 Key，再显式启用 Client；不能用一个权限全开、限额全空的隐式默认策略直接上线。策略创建默认建议为 `scopes=[]`、`allow_mimic=false`、模型不限但没有路由 scope，因此默认拒绝所有数据调用。启用操作应单独提供并写管理审计。

### 16.2 数据模型

推荐 PostgreSQL；单进程本机、低流量的 Phase A-C 可用 SQLite，但必须开启 WAL、busy timeout 和外键。生产 Phase D Token 硬额度和 Phase E 多副本必须使用 PostgreSQL 或另一种经过并发/崩溃测试的事务型共享存储，不能用 SQLite/NFS 文件冒充共享数据库。不要把 Client Key 或用量计数混进 Claude OAuth 凭据 JSON 文件。

#### `clients`

```sql
CREATE TABLE clients (
  id               UUID PRIMARY KEY,
  name             VARCHAR(100) NOT NULL,
  status           VARCHAR(20) NOT NULL DEFAULT 'disabled' CHECK (status IN ('active','disabled')),
  description      VARCHAR(500),
  created_at       TIMESTAMPTZ NOT NULL,
  updated_at       TIMESTAMPTZ NOT NULL,
  disabled_at      TIMESTAMPTZ
);
```

#### `client_keys`

```sql
CREATE TABLE client_keys (
  id               UUID PRIMARY KEY,
  client_id        UUID NOT NULL REFERENCES clients(id),
  name             VARCHAR(100) NOT NULL,
  lookup_id        VARCHAR(32) NOT NULL,
  display_prefix   VARCHAR(40) NOT NULL,
  secret_hash      BYTEA NOT NULL,
  hash_version     SMALLINT NOT NULL DEFAULT 1,
  status           VARCHAR(20) NOT NULL CHECK (status IN ('active','revoked')),
  expires_at       TIMESTAMPTZ,
  last_used_at     TIMESTAMPTZ,
  created_at       TIMESTAMPTZ NOT NULL,
  revoked_at       TIMESTAMPTZ,
  UNIQUE(lookup_id)
);
```

Key 格式建议：

```text
ccgw_<public-key-id>_<32 random bytes base64url-no-pad>
```

`public-key-id` 是至少 96-bit 随机、base64url 编码的查找标识，不是数据库自增 ID；解析时限制字符集和长度。创建时只返回一次完整 Key；数据库保存 `lookup_id`、固定长度 `display_prefix` 和 `HMAC-SHA256(key_pepper, full_key)`，不保存明文。这里不需要慢密码哈希，因为 secret 是 256-bit 随机秘密；HMAC pepper 来自环境/Secret，必须与数据库分离。先按 lookup ID 取得单行候选，再使用常量时间函数比较 HMAC。日志只显示 `display_prefix`，不能把完整 Key 放进缓存键、trace attribute 或错误。

Key pepper 轮换需要 `hash_version`：验证期同时加载当前和上一版本 pepper；管理员逐把轮换 Client Key 后移除旧 pepper。丢失 pepper 等于所有托管 Key 失效，必须有备份和轮换手册。

#### `client_policies`

每个 Client 一行当前策略；修改时增加 `version`，请求认证后捕获不可变快照：

```sql
CREATE TABLE client_policies (
  client_id                  UUID PRIMARY KEY REFERENCES clients(id),
  version                    BIGINT NOT NULL,
  scopes                     JSONB NOT NULL,
  allowed_models             JSONB,
  denied_models              JSONB,
  allow_mimic                BOOLEAN NOT NULL DEFAULT FALSE,
  rpm_limit                  INTEGER,
  concurrent_limit           INTEGER,
  input_tpm_limit            BIGINT,
  output_tpm_limit           BIGINT,
  total_tpm_limit            BIGINT,
  fixed_input_reservation    BIGINT,
  max_output_tokens_request  INTEGER,
  daily_total_tokens         BIGINT,
  monthly_total_tokens       BIGINT,
  lifetime_total_tokens      BIGINT,
  timezone                   VARCHAR(64) NOT NULL DEFAULT 'UTC',
  updated_at                 TIMESTAMPTZ NOT NULL
);
```

限额字段统一语义：`rpm_limit`、`concurrent_limit`、三个 TPM、`max_output_tokens_request` 和三个周期额度均为 `NULL=不限制`、`0=禁止该能力/不允许任何用量`、正数为硬上限。`fixed_input_reservation` 不是限额：`NULL` 或 `0` 表示不做固定预留，正数表示每次请求保守预占的 input Token。API 不接受任何负数，也不能沿用原项目中“0=不限”的限额语义。scope 当前只允许：

```text
messages:write
tokens:count
models:read
usage:read_self
```

`allowed_models=NULL` 表示不额外限制，`allowed_models=[]` 表示拒绝所有模型，非空数组表示 allowlist；实现必须保留 NULL 与空数组的差异。`denied_models=NULL/[]` 都表示无 deny，其他情况 deny 总是优先。模型权限先匹配客户端请求的原始模型名，再解析 alias，并对映射后模型再检查一次，防止别名绕过。首版只支持精确模型 ID，不支持任意正则；如需系列匹配，只允许经验证的 glob，并给出最大规则数/长度。managed 模式的 `/v1/models` 也要按同一 Policy 过滤配置目录，但目录仍不是完整上游能力清单。

Policy 和所有 RPM、并发、TPM、周期额度均以 `client_id` 聚合，多把 Key 只是凭据轮换和设备审计手段，不产生独立额度。`usage_requests.client_key_id` 可用于归因和管理员筛选，但不能让调用方通过创建多把 Key 成倍获得限额。网关级全局并发/连接上限仍独立生效，用于保护单个上游 OAuth 账号和进程资源。

#### `usage_requests`

一条逻辑调用一行，是审计和幂等结算的事实表：

```sql
CREATE TABLE usage_requests (
  id                       UUID PRIMARY KEY,
  request_id               VARCHAR(64) NOT NULL UNIQUE,
  client_id                UUID NOT NULL REFERENCES clients(id),
  client_key_id            UUID NOT NULL REFERENCES client_keys(id),
  policy_version           BIGINT NOT NULL,
  route                    VARCHAR(64) NOT NULL,
  requested_model          VARCHAR(128),
  upstream_model           VARCHAR(128),
  stream                   BOOLEAN,
  status                   VARCHAR(24) NOT NULL,
  gateway_status           INTEGER,
  upstream_status          INTEGER,
  input_tokens             BIGINT NOT NULL DEFAULT 0,
  output_tokens            BIGINT NOT NULL DEFAULT 0,
  cache_creation_tokens    BIGINT NOT NULL DEFAULT 0,
  cache_read_tokens        BIGINT NOT NULL DEFAULT 0,
  quota_input_tokens       BIGINT NOT NULL DEFAULT 0,
  quota_output_tokens      BIGINT NOT NULL DEFAULT 0,
  quota_total_tokens       BIGINT NOT NULL DEFAULT 0,
  quota_source             VARCHAR(24),
  counts_toward_quota      BOOLEAN NOT NULL DEFAULT FALSE,
  reserved_input_tokens    BIGINT NOT NULL DEFAULT 0,
  reserved_output_tokens   BIGINT NOT NULL DEFAULT 0,
  usage_source             VARCHAR(24),
  usage_complete           BOOLEAN NOT NULL DEFAULT FALSE,
  started_at               TIMESTAMPTZ NOT NULL,
  first_token_at           TIMESTAMPTZ,
  completed_at             TIMESTAMPTZ,
  reject_stage             VARCHAR(32),
  error_class              VARCHAR(48),
  upstream_request_id      VARCHAR(128)
);

CREATE INDEX usage_requests_client_time
  ON usage_requests(client_id, started_at DESC);
```

建议状态：`accepted`、`forwarding`、`completed`、`gateway_error`、`upstream_error`、`client_cancelled`、`stream_incomplete`、`local_rejected`。`accepted` 表示请求已通过本地检查并写入事实表，不代表一定启用了 Token Reservation；`gateway_error` 表示 Begin 之后、上游明确处理之前发生 OAuth/构造/本地存储错误。状态机只允许 `accepted -> forwarding|gateway_error|client_cancelled`，`forwarding -> completed|upstream_error|client_cancelled|stream_incomplete`；`local_rejected` 和其他终态不能再次转换。`reject_stage/error_class` 只能写低基数枚举，例如 `scope/permission_denied`、`rpm/rate_limited`、`body/invalid_json`，不能写原始错误、Key、Prompt 或请求片段。不要保存 Prompt、消息正文、System、工具参数或原始响应。

`input/output/cache_*_tokens` 永远表示从 Anthropic 响应中实际观测到的 usage；`quota_*_tokens` 表示本地额度账本最终采用的数值。`quota_source` 只允许 `actual/reserved/adjustment/none`。完整成功响应时 `quota_input=input+cache_creation+cache_read`、`quota_output=output`、`quota_total=quota_input+quota_output`；没有最终 usage 的中断流，实际观测字段保留已知值且 `usage_complete=false`，而 `quota_*` 可按 reservation 保守结算。绝不能把预占估算写进实际观测字段。`counts_toward_quota=false` 用于 `count_tokens`、`models` 和 `local_rejected`；这些请求仍可计请求数，但不进入推理 Token 桶。

从 `capability_level=usage` 开始，`local_rejected` 的账本口径必须固定；Phase A/B 尚未启用 usage ledger 时执行相同分类，但只写脱敏结构化日志和低基数指标，查询 API 明确返回该能力未启用：

- Key 无法认证时还不知道可信的 Client，不创建 Client usage 行，只增加无 Key 标签的认证失败指标，并由独立防爆破 limiter 处理。
- Key 与 Client 认证成功后，scope、Body、model、mimic、`max_tokens`、RPM、并发或额度检查造成的本地拒绝，Phase C+ 尝试写一条 `local_rejected`，保存 `client_id/key_id/policy_version/gateway_status/reject_stage/error_class`，Token 全为 0，不创建或占用 Token Reservation。
- `local_rejected` 计入 `requests/rejected_requests`，但不计入推理 Token 用量和周期 Token 额度。用量 API 必须把 `forwarded_requests` 与 `local_rejected_requests` 分开返回。
- 写本地拒绝审计失败绝不能反过来放行上游；保留原本 400/403/429/503 响应并增加告警指标。若失败原因本身是硬限量存储不可用，仍然 fail closed 返回 503。
- 对同一 `request_id` 的拒绝写入必须幂等。生产环境还应在入口增加独立的 IP/连接防护，避免持有有效 Key 的攻击者用大量被拒请求制造数据库写放大。

#### `quota_reservations` 与聚合桶

为硬 Token 限量建立独立预占。不要只放一个 `reserved_tokens`，因为 input、output、total TPM 是三个独立维度。推荐规范化结构：

```sql
CREATE TABLE quota_reservations (
  id               UUID PRIMARY KEY,
  request_id       VARCHAR(64) NOT NULL UNIQUE REFERENCES usage_requests(request_id),
  client_id        UUID NOT NULL REFERENCES clients(id),
  policy_version   BIGINT NOT NULL,
  status           VARCHAR(16) NOT NULL CHECK (status IN ('active','settled','released','expired')),
  expires_at       TIMESTAMPTZ NOT NULL,
  heartbeat_at     TIMESTAMPTZ NOT NULL,
  created_at       TIMESTAMPTZ NOT NULL,
  settled_at       TIMESTAMPTZ
);

CREATE TABLE usage_buckets (
  client_id        UUID NOT NULL REFERENCES clients(id),
  bucket_kind      VARCHAR(16) NOT NULL CHECK (bucket_kind IN ('minute','day','month','lifetime')),
  bucket_start     TIMESTAMPTZ NOT NULL,
  dimension        VARCHAR(16) NOT NULL CHECK (dimension IN ('input','output','total')),
  used_tokens      BIGINT NOT NULL DEFAULT 0 CHECK (used_tokens >= 0),
  reserved_tokens  BIGINT NOT NULL DEFAULT 0 CHECK (reserved_tokens >= 0),
  version          BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (client_id, bucket_kind, bucket_start, dimension)
);

CREATE TABLE quota_reservation_items (
  reservation_id   UUID NOT NULL REFERENCES quota_reservations(id),
  bucket_kind      VARCHAR(16) NOT NULL,
  bucket_start     TIMESTAMPTZ NOT NULL,
  dimension        VARCHAR(16) NOT NULL,
  reserved_tokens  BIGINT NOT NULL CHECK (reserved_tokens >= 0),
  PRIMARY KEY (reservation_id, bucket_kind, dimension)
);
```

`quota_reservation_items` 固化本次请求实际占用的桶和维度，settle/release 必须按这些键减预占，不能按结算时的当前时间或新 Policy 重新推导。minute 可有 input/output/total 三项；day/month/lifetime 首版只需 total。`usage_buckets` 的 `used_tokens` 是本地 quota-accounted 数值，不等于一定完整的上游观测 usage。

日/月窗口使用请求开始时 PolicySnapshot 的 IANA timezone，保存的 `bucket_start` 仍为 UTC instant；timezone 修改只对新请求的新窗口生效，不能重解释历史桶。累计 lifetime 使用固定 epoch bucket。首版 TPM 使用请求开始时间归属的固定分钟桶，不宣称为滚动 60 秒窗口，也无法按 SSE 每个 Token 的真实产生时刻归属。

Phase D 首选让 `usage_requests`、reservation、reservation items 和 buckets 全部以 PostgreSQL 为权威源，在一个事务内原子写入；Redis 只用于 RPM 和并发 lease。不能在 PostgreSQL 插 usage、再在 Redis 预占 Token 后声称具备原子硬额度。若未来因吞吐改用其他存储，必须提供单一原子权威源或经过证明的事务/outbox 恢复协议，并完成崩溃点故障注入。

管理审计与手工调整也应独立建表：

```text
admin_audit_logs(id, actor_fingerprint, action, target_type, target_id,
                 request_id, before_digest, after_digest, created_at)
usage_adjustments(id, client_id, request_id nullable, dimension, delta_tokens,
                  reason, actor_fingerprint, created_at)
```

`usage_adjustments` 允许负 delta，但应用后 bucket 不得小于 0；写 adjustment 与修改相关 bucket 必须同事务。摘要只保存规范化非敏感字段的 hash/版本，不能保存 Key、Token 或完整请求 Body。

### 16.3 管理 API

全部只绑定 `admin_listen`，使用 `ADMIN_API_KEY`、`Cache-Control: no-store`、严格 JSON 大小限制和低速率上限。

```text
POST   /admin/clients
GET    /admin/clients
GET    /admin/clients/{client_id}
PATCH  /admin/clients/{client_id}
POST   /admin/clients/{client_id}/disable
POST   /admin/clients/{client_id}/enable

POST   /admin/clients/{client_id}/keys
GET    /admin/clients/{client_id}/keys
POST   /admin/client-keys/{key_id}/revoke
POST   /admin/client-keys/{key_id}/rotate

GET    /admin/clients/{client_id}/policy
PUT    /admin/clients/{client_id}/policy
GET    /admin/clients/{client_id}/usage
GET    /admin/clients/{client_id}/limits
GET    /admin/usage/requests
```

创建 Client 示例：

```json
{
  "name": "coding-team-a",
  "description": "Claude Code gateway client"
}
```

创建 Key 成功仅此一次返回 secret：

```json
{
  "id": "<uuid>",
  "client_id": "<uuid>",
  "name": "laptop-2026-07",
  "key": "ccgw_<lookup>_<secret>",
  "prefix": "ccgw_<lookup>",
  "expires_at": null
}
```

后续 GET 只能返回 `id/name/prefix/status/expires_at/last_used_at`。`rotate` 在同一事务创建新 Key 并返回一次 secret；旧 Key 可配置立即撤销或设置短 overlap deadline。默认立即撤销，且认证缓存必须通过版本/短 TTL 在秒级失效。

策略更新示例：

```json
{
  "expected_version": 7,
  "scopes": ["messages:write", "tokens:count", "models:read", "usage:read_self"],
  "allowed_models": ["claude-sonnet-4-5", "claude-haiku-4-5"],
  "denied_models": [],
  "allow_mimic": false,
  "rpm_limit": 30,
  "concurrent_limit": 3,
  "input_tpm_limit": 120000,
  "output_tpm_limit": 30000,
  "total_tpm_limit": 140000,
  "fixed_input_reservation": null,
  "max_output_tokens_request": 8192,
  "daily_total_tokens": 1000000,
  "monthly_total_tokens": 20000000,
  "lifetime_total_tokens": null,
  "timezone": "Asia/Shanghai"
}
```

使用乐观锁：`expected_version` 不匹配返回 409。更新整行 policy 并将 version `+1`，不能逐字段产生短暂的半更新状态。Client 启停、Key 创建/轮换/撤销、Policy 修改、usage adjustment 均写管理审计：actor=`admin-key fingerprint`、action、target、旧/新版本摘要、时间、request_id；不写任何 secret。业务变更与 audit 行必须同一数据库事务提交，不能出现“Key 已撤销但无审计”或“有审计但操作回滚”。Admin Key 若仍是单一环境变量，actor fingerprint 只能区分密钥版本，不能识别具体管理员；需要人员归因时应接入独立管理员身份/mTLS，不能伪称环境变量 Key 已提供个人审计。

可选数据端自助接口：

```text
GET /v1/gateway/usage?from=<RFC3339>&to=<RFC3339>&granularity=hour|day
GET /v1/gateway/limits
```

只允许 scope `usage:read_self`，永远从认证上下文取 client_id，不能接受查询参数指定其他 Client。响应不包含上游 OAuth 信息或其他调用方数据。

自助 `usage/limits` 是控制面只读请求，必须能在 Client 推理 RPM/TPM/日月额度已耗尽时继续访问，否则调用方无法诊断为什么被拒。它仍检查 Key、Client 和 `usage:read_self`，但不消耗推理 RPM、不占推理并发、不进入 Token quota，也不为查询本身创建 `usage_requests`，避免递归统计；使用独立的低速率查询 limiter、最大时间范围、分页和数据库 statement timeout 防滥用。Client disabled 或 Key revoked 后仍拒绝自助查询，管理员可从 admin 接口查看历史。

### 16.4 认证和授权执行顺序

managed 模式每次数据请求：

```text
1. 严格解析唯一 Authorization/x-api-key（沿用第 6.3 节歧义拒绝规则）
2. 从 Key 的 public lookup ID 定位候选行
3. HMAC 完整 Key，常量时间比较 secret_hash
4. 检查 key active/expires_at、client active
5. 读取不可变 PolicySnapshot(version=N)
6. 消耗 1 个 Client RPM token；认证成功后的请求即使随后 Body 非法也计 RPM
7. 校验 route scope
8. 限制并读取 Body，取原始 model/max_tokens/stream
9. 校验 max_tokens、原始模型与 alias 后模型权限
10. 若不是官方 Claude Code：同时要求全局 mimic 开关和 policy.allow_mimic
11. 原子申请并发 lease
12. capability>=usage 时原子创建 usage_requests(status=accepted)；若启用 Token 硬额度，在同一事务完成 quota reservation
13. Phase A/B 在授权/实时限制通过后可直接继续；Phase C/D 只有第 12 步成功才进入 OAuth Token 获取和反代
```

第 6 步之后任一检查失败均按上一节的 `local_rejected` 能力规则处理；第 6 步自身被 RPM 拒绝也只分类一次，不能再消耗第二次 RPM。第 12 步遇到额度不足时，事务应写入 `local_rejected` 且不保留 Reservation；遇到存储故障则整个事务回滚并返回 503。并发 lease 在第 12 步失败时也必须释放。

拒绝语义：

| 场景 | HTTP/Anthropic 类型 |
|---|---|
| Key 缺失、错误、撤销或过期 | `401 authentication_error`，不泄露具体原因 |
| Client disabled | `403 permission_error` |
| scope/model/mimic 不允许 | `403 permission_error` |
| RPM/TPM/日月累计额度/并发超限 | `429 rate_limit_error`，带有界 `retry-after`（有确定重置时间时） |
| `max_tokens` 超出单请求上限 | `400 invalid_request_error`，提示允许最大值 |
| 强制限量存储不可用 | `503 api_error`，fail closed |

不要为“方便”把 Client Key 当管理员 Key。管理路由在认证链上先按监听地址分离，再只接受 Admin Key；即使 Client Policy 拥有所有 scope 也不能访问 `/admin/*`。

`last_used_at` 不是关键账本，可以按 30-60 秒节流异步更新；Key 撤销和 Client 禁用是安全关键写，必须同步提交并立即失效本机认证缓存。缓存 entry 必须带 client status、key status、expires_at、policy version；正缓存短 TTL，错误 Key 仅做极短负缓存，避免攻击者造成无界内存。

### 16.5 限量算法与准确性边界

#### RPM

建议使用 token bucket 或精确滑动窗口，不建议简单的“自然分钟计数”，后者会在分钟边界允许双倍突发。单进程可用带互斥的内存 bucket；多副本必须用 Redis Lua/具备原子性的共享存储。

```text
capacity = rpm_limit
refill_rate = rpm_limit / 60 tokens per second
每个进入反代候选的请求消耗 1 token
```

本地 JSON 校验错误是否计 RPM 必须固定。本规格规定：完成 Key/Client 认证并读取 PolicySnapshot 后立即计数，随后发生的 scope、JSON 或模型错误仍占 1 次 RPM，防止攻击者用无效 Body 绕过；Key 认证失败由独立的认证防爆破 limiter 处理，不计 Client RPM。

#### 并发

在所有前置校验通过后、获取上游 OAuth Token 之前原子申请一个 Client 槽。非流式在响应完成时释放；SSE 从上游请求开始一直占用到 EOF、客户端取消、idle timeout 或强制关闭。所有 return/panic 路径必须 `defer Release()`。

单进程用内存 semaphore 即可。多副本使用带 lease ID/TTL/heartbeat 的共享集合；只设置 TTL 不续租会让长 SSE 提前释放并超卖。限量后端不可用时，配置了硬并发上限的 Client 必须 fail closed。

#### 单请求输出上限

若 `max_output_tokens_request` 非 NULL：请求必须包含合法正整数 `max_tokens`，且不得超过上限。网关默认拒绝而不是静默截小，因为改写会改变调用语义；可选 clamp 必须显式配置并在响应 Header 标记，但不建议用于 Claude Code。

#### TPM 和周期 Token 限额

上游实际 input/output/cache token 只有响应后才知道，所以严格限额需要预占。推荐：

```text
reserved_output = request.max_tokens
estimated_input = 本地 tokenizer 估算值（若未实现可靠 tokenizer，则为 0 且 input 硬限量不承诺严格）
reserve_total   = estimated_input + reserved_output

原子判断：used + active_reserved + reserve_delta <= limit
成功：增加 minute/day/month/lifetime bucket.reserved_tokens
失败：不发上游，返回 429

收到最终 usage：
  actual_total = input + output + cache_creation + cache_read
  原子执行 reserved -= reserve_total; used += actual_total; reservation=settled
```

实际 usage 可能高于预占值或使 `used_tokens` 超过当前策略上限，例如 input 估算不足、上游返回量异常或管理员在请求途中降低限额。此时 settle 必须完整增加实际 Token、允许 bucket 暂时超过 limit，并在限量视图标记 `over_limit=true`；不能把 actual 截断到 reservation/limit，也不能因 CHECK 失败回滚已发生的消耗。该 Client 的后续受限请求立即拒绝，直到窗口重置或管理员通过有审计的 adjustment 修正。预占只控制发起请求时的准入，不是最终用量的上界。

输入、输出、总 TPM 若分别配置，应分别维护预占维度；不能拿 output reservation 同时假装约束 input TPM。`count_tokens` 成功返回的 `input_tokens` 记录为辅助事件，默认不计入推理 Token 配额，因为它不是 `/messages` 推理消耗；是否计 RPM 单独配置，建议计 1 次请求。

一个重要限制：即使预占 `max_tokens`，实际 input token 仍可能使总量超过剩余额度。要宣称 input/total 是“硬限制”，必须在转发前得到可信输入估算，可调用本地与 Claude 模型兼容的 tokenizer；不要为每个请求再调用上游 `count_tokens`，这会增加延迟、配额和故障面。没有可靠估算时：

- RPM、并发和 `max_tokens` 可以严格执行。
- output TPM/周期 output 限额可通过 `max_tokens` 预占近似严格执行。
- input/total Token 限额只能是事后限流，必须在 UI/API 标记 `enforcement_mode=postpaid`，不能承诺零超额。

为减小超卖，策略可设置 `fixed_input_reservation` 固定保守预留，或拒绝剩余额度低于安全余量的请求。固定预留只减少并发超卖，不会变成可信 tokenizer，因此 input/total 的 `enforcement_mode` 仍应标为 `conservative` 或 `postpaid`，不能标为精确硬限制。不要把字符数/4 当作精确 Token 数。

#### 预占释放和孤儿回收

| 结果 | 处理 |
|---|---|
| 上游请求尚未发出即本地失败 | 全量 release，不计实际 token |
| 上游明确 4xx 且无 usage | release；usage_requests 记录 upstream_error |
| 成功 JSON 有 usage | settle actual |
| SSE 正常终止且最终 event 有 usage | settle actual |
| 客户端取消但网关已看到最终 usage | settle actual |
| 客户端取消/网络断开且无最终 usage | 标记 `stream_incomplete`，默认保留 reserved amount 作为保守结算 |
| 网关崩溃留下 active reservation | 后台回收器在过期后按 `orphan_policy` 处理 |

“尚未发出”必须是网关可以证明还没有调用 `RoundTrip`，例如 OAuth 获取失败、构造请求失败或本地取消。进入 `RoundTrip` 后发生 timeout、EOF、连接重置或 5xx 且没有最终 usage，无法证明 Anthropic 未处理；应标记 incomplete，并按 `charge_reserved` 保守结算。只有协议明确、经过测试的拒绝响应（如 400/401/403/404/409/429 且无 usage）才 release。不能仅凭客户端没收到响应或 Go transport 返回 error 就释放。

硬限量默认 `orphan_policy=charge_reserved`，否则客户端可反复断开逃避额度。管理员可手工核销误差，但所有调整必须追加审计行，不能直接改历史 usage。Reservation TTL 必须大于允许的最长 SSE 时长，并由活跃流 heartbeat 延长；不能让仍在运行的流被回收。

### 16.6 用量采集与账本语义

Anthropic Messages 非流式响应通常在顶层 `usage` 返回：

```json
{
  "usage": {
    "input_tokens": 120,
    "output_tokens": 45,
    "cache_creation_input_tokens": 20,
    "cache_read_input_tokens": 80
  }
}
```

SSE 至少从 `message_start.message.usage` 和 `message_delta.usage` 合并，不能对每个 event 简单相加造成重复。聚合器要按字段“最新累计值/协议定义”处理，并用 fixture 覆盖真实事件序列。保留原始上游 usage 字段的已知数值，不从文本重新估算已提供的数据。

统一内部模型：

```go
type TokenUsage struct {
	InputTokens          int64
	OutputTokens         int64
	CacheCreationTokens  int64
	CacheReadTokens      int64
	Complete             bool
	Source               string // anthropic_json | anthropic_sse | unavailable
}
```

“总 Token”必须明确定义。建议限量使用：

```text
total_tokens = input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens
```

同时在 API 中逐项返回，避免不同人对 cache token 是否包含在 input 中理解不同。若未来 Anthropic 字段语义变化，增加 `usage_schema_version`，不能静默改变历史统计公式。

账本规则：

1. `request_id` 由网关生成，客户端提供的 `x-client-request-id` 只作关联，不作为数据库幂等主键。
2. 同一 request/reservation 只能 settle 一次；401 Token 刷新后的单次重试仍属于同一逻辑 request。
3. 网络错误/5xx 默认不重试的原因也适用于用量：上游可能已处理但未返回 usage，应标记 `usage_complete=false`，不能记成确定的 0。
4. `usage_requests` 是 append-mostly 事实记录，只允许状态机定点更新；管理员调整写独立 `usage_adjustments`，包括 delta、reason、actor 和 created_at。
5. usage 和 quota bucket 的 settle 必须在同一事务/原子脚本中完成。异步日志队列可以提高吞吐，但硬额度结算不能丢；队列满或数据库不可用时进入 fail closed/degraded，而不是 drop。
6. 禁止用浮点数保存 Token；全部用 `BIGINT`。本阶段不计算美元成本，也不展示“余额”。

建议用量查询响应：

```json
{
  "client_id": "<uuid>",
  "from": "2026-07-01T00:00:00Z",
  "to": "2026-07-02T00:00:00Z",
  "totals": {
    "requests": 42,
    "forwarded_requests": 39,
    "local_rejected_requests": 3,
    "input_tokens": 12345,
    "output_tokens": 2345,
    "cache_creation_tokens": 100,
    "cache_read_tokens": 5000,
    "incomplete_requests": 1,
    "quota_accounted_total_tokens": 28022
  },
  "limits": {
    "daily_total_tokens": 1000000,
    "daily_used_tokens": 19790,
    "daily_reserved_tokens": 8192,
    "daily_resets_at": "2026-07-02T16:00:00Z",
    "enforcement_mode": "reserved"
  }
}
```

这里的 `input/output/cache_*` 是上游实际观测值，`quota_accounted_total_tokens` 是包含保守结算和 adjustment 后的本地额度值，两者允许不同。API 还应返回 `usage_complete`/`usage_source` 或按完整性分组，不能把 incomplete 的未知 Token 当作 0 后宣称统计完整。Phase C 尚未启用 Token quota 时，`limits` 返回 `capability="usage"` 和 `enforcement_mode="not_enabled"`，不得伪造 used/reserved/reset；Phase A/B 尚未实现用量查询时，相应 endpoint 返回 404 或 501 的固定能力错误，不返回空统计冒充真实 0。

查询必须限制时间范围、page size 和 granularity；明细使用 cursor 分页，不能允许无界导出拖垮数据库。默认保留期例如 90 天应配置化；删除/聚合前必须先满足审计和业务需求。

### 16.7 完整请求处理顺序

启用控制面后，`/v1/messages` 的完整顺序替代第 9 节的简版顺序：

```text
1. 生成 gateway request_id
2. Client Key 认证并检查 Client/Key 状态，取得 immutable PolicySnapshot
3. RPM consume；成功认证后的每个数据请求只消费一次
4. 校验 route scope
5. 限制并读取 Body，拒绝歧义 JSON
6. 解析原始 model/max_tokens/stream，校验单请求、模型与模拟权限
7. 获取 Client concurrency lease（立即 defer 幂等 release）
8. Phase C+ 原子创建 usage request；Phase D 在同一事务附带 quota reservation
9. 识别真实 Claude Code，执行允许的模拟/定点 Body 处理
10. 获取或刷新 Claude OAuth access token
11. 构造并发送固定 Anthropic 上游请求
12. 401 时在同一 request/reservation 下最多刷新重试一次
13. JSON/SSE 转发，同时收集最终 usage
14. Phase C+ 使用独立有界 Context 结算 usage request；Phase D 同事务 settle/release reservation 和 buckets
15. 释放并发 lease，输出脱敏结构化日志
```

关键不变量：Phase A/B 在授权和实时限制通过后即可发上游；它们必须在能力 API 中明确声明没有完整 usage ledger。Phase C/D 在第 8 步完成之前不能向 Anthropic 发请求，否则存储故障会产生未记账请求。第 3-8 步的本地拒绝按第 16.2 节能力规则处理。第 14 步不能沿用已被客户端取消的请求 Context，应使用 `context.WithoutCancel` 加短超时；结算失败时上游消耗已经发生，必须停止接受该 Client 的新请求并幂等重试，不能简单释放额度或丢弃事件。

`/v1/messages/count_tokens` 同样认证/scope/RPM/并发和记请求，但不做 output reservation；成功时记录返回的 input token 作为 `route=count_tokens` 指标。`/v1/models` 只做 Key/scope/RPM，可选择不占并发槽和 Token quota，但必须记 request count。

### 16.8 多副本与失败策略

单进程内存实现只适合单副本。只要启用多副本：

- Clients、Keys、Policy、usage ledger 使用共享数据库。
- RPM/并发/Reservation 使用共享、原子存储，或全部在数据库事务内实现。
- Key/Policy 缓存失效使用消息通知加短 TTL；通知不是正确性的唯一来源。
- 管理写提交后必须让旧 policy version 在有界时间失效。
- Reservation 回收器使用 leader lock 或 `FOR UPDATE SKIP LOCKED`，避免重复处理。
- 所有时间由服务端 UTC 时钟产生，主机必须同步 NTP；客户端时间不参与窗口归属。

失败模式建议：

| 子系统 | 无硬限制 Client | 配置硬限制 Client |
|---|---|---|
| Key/Policy DB 不可用且无有效缓存 | fail closed 503 | fail closed 503 |
| RPM/并发存储不可用 | 可配置 fail open，但必须告警 | fail closed 503 |
| Usage/Reservation 存储不可用 | Phase C+ 默认 fail closed 503；否则不能保证完整账本 | fail closed 503 |
| 非关键 last_used/audit 异步更新失败 | 请求可继续，重试并告警 | 请求可继续，重试并告警 |

不要照搬原项目 RPM 的 fail-open 选择后仍称其为“硬限量”。管理 API 和 `/readyz` 应分别暴露低基数状态 `auth_store_degraded`、`limit_store_degraded`、`usage_store_degraded`，不泄露 DSN 或 SQL。

### 16.9 核心 Go 接口与事务边界

下面是建议的领域边界，不要求逐字照抄命名，但实现不能把数据库、Redis、HTTP Handler 和上游转发揉在一起。所有 ID 示例使用 string 是为了不绑定 UUID 库；正式实现应统一使用强类型 UUID。

```go
type ClientPrincipal struct {
	ClientID  string
	KeyID     string
	KeyPrefix string // 只用于脱敏日志，绝不保留完整 Key
}

type PolicySnapshot struct {
	ClientID   string
	Version    int64
	Scopes     map[string]struct{}
	AllowModel map[string]struct{} // nil 表示不额外限制
	DenyModel  map[string]struct{}
	AllowMimic bool

	RPMLimit                 *int
	ConcurrentLimit          *int
	InputTPMLimit            *int64
	OutputTPMLimit           *int64
	TotalTPMLimit            *int64
	FixedInputReservation    *int64
	MaxOutputTokensPerRequest *int
	DailyTotalTokens         *int64
	MonthlyTotalTokens       *int64
	LifetimeTotalTokens      *int64
	Timezone                 string
}

type AuthContext struct {
	Principal ClientPrincipal
	Policy    PolicySnapshot // 当前请求捕获的不可变副本
}

type ClientAuthenticator interface {
	Authenticate(ctx context.Context, presentedKey string, now time.Time) (AuthContext, error)
	InvalidateKey(ctx context.Context, keyID string) error
	InvalidateClient(ctx context.Context, clientID string) error
}

type PolicyStore interface {
	GetSnapshot(ctx context.Context, clientID string) (PolicySnapshot, error)
	Replace(ctx context.Context, clientID string, expectedVersion int64, next PolicyInput) (PolicySnapshot, error)
}
```

接口约束：

- `Authenticate` 内部解析 lookup ID、查询候选、计算 HMAC、常量时间比较、检查 Key/Client 状态并取得 PolicySnapshot；完整 Key 不能进入返回值、Context、缓存键、错误或日志。
- 认证缓存的键使用 lookup ID 加 Key HMAC，不使用完整 Key；entry 带 Key/Client 状态、过期时间和 policy version。撤销/禁用提交成功后立即清本机缓存，并用短 TTL 约束跨副本陈旧窗口。
- `PolicySnapshot` 捕获后不可原地修改。管理员更新只影响后续请求；流式请求不在中途切换策略，但已取得的并发 lease 和 reservation 仍须正常结算。
- `Replace` 必须以 `expectedVersion` 做单条条件更新；未命中返回可用 `errors.Is` 判断的 `ErrVersionConflict`，不能比较错误字符串。
- 所有 nullable limit 在 Go 中使用指针或显式 Optional 类型，保留 `NULL`、`0`、正数三种含义。

RPM 和并发属于实时限制：

```go
type RateDecision struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration // 不能确定时为 0，不伪造 reset 时间
}

type ConcurrencyLease interface {
	ID() string
	Lost() <-chan error              // 续租失败或 lease 丢失时通知调用方
	Release(ctx context.Context) error // 必须幂等
}

type LimitStore interface {
	ConsumeRPM(ctx context.Context, clientID string, limit int, now time.Time) (RateDecision, error)
	AcquireConcurrency(
		ctx context.Context,
		clientID string,
		limit int,
		requestID string,
		ttl time.Duration,
	) (ConcurrencyLease, RateDecision, error)
}
```

- `LimitStore` 的实现自己负责原子 acquire、TTL 和 heartbeat；不能由 Handler 用“先 count 后 insert”拼装。
- 成功取得 lease 后立即注册 `defer`，释放使用脱离客户端取消但有短 timeout 的 Context。`Release` 多次调用必须无害。
- hard concurrency 下 lease 的 heartbeat 连续失败时，`Lost()` 必须使上游请求被取消并告警；不能继续无限流式传输却声称并发上限仍然严格。
- `limit=NULL` 时调用方不调用对应接口；`limit=0` 直接拒绝，不把 0 传给一个约定为“禁用限制”的旧实现。

Token 额度和用量事实必须共用一个原子边界。建议将其命名为 `UsageStore` 或 `MeteringStore`：

```go
type Reservation struct {
	ID                   string
	RequestID            string
	ClientID             string
	PolicyVersion        int64
	ReservedInputTokens  int64
	ReservedOutputTokens int64
	ExpiresAt            time.Time
}

type BeginRequestCommand struct {
	RequestID       string
	ClientPrincipal ClientPrincipal
	Policy          PolicySnapshot
	Route           string
	RequestedModel  string
	UpstreamModel   string
	Stream          bool
	EstimatedInput  int64
	RequestedOutput int64
	StartedAt       time.Time
}

type BeginRequestDecision struct {
	Allowed     bool
	Reservation *Reservation // 未启用 Token 硬额度时允许为 nil
	LimitKind   string       // input_tpm|output_tpm|total_tpm|daily|monthly|lifetime
	RetryAfter  time.Duration
}

type LocalReject struct {
	RequestID      string
	Principal      ClientPrincipal
	PolicyVersion  int64
	Route          string
	RequestedModel string
	GatewayStatus  int
	RejectStage    string
	ErrorClass     string
	At             time.Time
}

type SettleCommand struct {
	RequestID   string
	Reservation *Reservation // Phase C 可为 nil；Phase D 启用预占后必须非 nil
	Status      string
	Usage       TokenUsage
	HTTPStatus  int
	CompletedAt time.Time
}

type UsageQuery struct {
	ClientID string // 自助接口必须由认证上下文写入
	From     time.Time
	To       time.Time
	Cursor   string
	PageSize int
}

type UsageStore interface {
	RecordLocalReject(ctx context.Context, in LocalReject) error
	Begin(ctx context.Context, in BeginRequestCommand) (BeginRequestDecision, error)
	MarkForwarding(ctx context.Context, requestID, upstreamRequestID string) error
	HeartbeatReservation(ctx context.Context, reservationID string, extendTo time.Time) error
	ReleaseBeforeUpstream(ctx context.Context, requestID string, reservation *Reservation, reason string, at time.Time) error
	Settle(ctx context.Context, in SettleCommand) error
	SettleIncomplete(ctx context.Context, in SettleCommand) error
	Query(ctx context.Context, in UsageQuery) (UsagePage, error)
	GetLimits(ctx context.Context, clientID string, now time.Time) (LimitView, error)
}
```

`UsageStore.Begin` 是最重要的事务边界。Phase C 尚未启用 Token 硬额度时，它只幂等插入 `usage_requests(accepted)` 并返回 `Allowed=true, Reservation=nil`。Phase D 启用任一 Token 硬额度后，它必须在一个数据库事务或一个原子脚本内完成：

```text
锁定/原子读取 minute、day、month、lifetime bucket
-> 检查 used + reserved + delta
-> 若允许：插入 usage_requests(accepted) + reservation + 增加各桶 reserved
-> 若额度不足：插入 usage_requests(local_rejected)，不增加 reserved，返回 Allowed=false
-> 提交
```

额度不足是成功提交后的 `Allowed=false`，不是返回一个会让事务回滚的普通 error；存储/事务故障才返回 error 并映射为 503。`Settle`、`SettleIncomplete` 和 `ReleaseBeforeUpstream` 以 request ID 和可选 reservation ID 幂等，重复调用不得二次扣量。Phase C 只更新请求事实；Phase D 必须在同一事务中同时更新事实、reservation 和 bucket。不能把硬额度操作拆成一个 best-effort `UsageRepository` 和另一个 `QuotaRepository`，也不能先异步写 usage 再更新 bucket。

`HeartbeatReservation` 只更新 active reservation 的 `heartbeat_at/expires_at`，不能改预占量或桶归属；条件更新未命中表示 reservation 已被结算/回收，调用方必须取消上游流并告警。长 SSE 每隔 `reservation_heartbeat_interval` 调用一次，使用独立短 timeout Context；连续失败达到有界阈值时也必须取消上游，不能让 reaper 与活跃请求同时结算。reaper 只处理 `heartbeat_at < now-orphan_grace` 且 `expires_at < now` 的 active 行，并用行锁/状态条件更新保证单次处理。

Handler 只负责协议编排，建议再提供应用层服务：

```go
type Authorizer interface {
	AuthorizeRoute(policy PolicySnapshot, scope string) error
	AuthorizeModels(policy PolicySnapshot, requested, upstream string) error
	AuthorizeMimic(policy PolicySnapshot, officialClaudeCode bool, globalEnabled bool) error
	ValidateMaxTokens(policy PolicySnapshot, maxTokens *int) error
}

type UsageCollector interface {
	FromJSON(body []byte) (TokenUsage, error)
	ObserveSSE(event []byte) error
	Final() TokenUsage
}
```

`Authorizer` 是纯函数、无数据库访问，便于穷举测试。`UsageCollector` 必须由 JSON/SSE fixture 驱动，不得在 Handler 中用字符串搜索 usage 字段。HTTP 层只把领域错误映射为 Anthropic error envelope；Repository 不直接写 HTTP 响应。

### 16.10 开发分期与每期交付

不要一次性把认证、统计和硬额度全部上线。按下面顺序开发，每一期都应可单独回滚，且上一期测试通过后再进入下一期。

#### Phase A：Client、Key 与权限

实现内容：

1. 建立 `clients/client_keys/client_policies/admin_audit_logs` 表和显式 migration。
2. 完成 Client CRUD、Key 一次性创建/轮换/撤销/过期、pepper version。
3. 实现 managed 认证、短缓存、Client 禁用、Key 撤销即时失效。
4. 实现 route scope、原始模型与 alias 后模型双重检查、`allow_mimic`。
5. 实现 `/v1/gateway/limits` 的策略只读视图；本期不返回伪造的实时用量。

验收门槛：数据库和日志搜不到明文 Key；权限矩阵全覆盖；Admin/Client Key 不能互换；static/managed 不存在双通道绕过。

#### Phase B：RPM、并发与单请求限制

实现内容：

1. token bucket RPM 及认证失败独立防爆破 limiter。
2. SSE 全生命周期并发 lease、heartbeat、幂等释放和优雅关机清理。
3. `max_output_tokens_request` 严格校验。
4. 标准 429、合法 `retry-after`、低基数指标和故障策略。

验收门槛：并发压测不超卖；自然分钟边界没有双倍突发；Redis/DB 故障时硬限制 fail closed；取消、panic、EOF 均不泄漏 lease。

#### Phase C：请求账本与用量查询

实现内容：

1. 建立 `usage_requests`，记录成功、上游错误、流中断和认证成功后的本地拒绝。
2. 实现 JSON/SSE `TokenUsage` 收集、幂等 request 状态机、明细保留和聚合查询。
3. 实现管理员查询与 `usage:read_self` 自助查询，增加范围、分页和超时上限。
4. 加入账本延迟、incomplete 数量、写失败等告警。

验收门槛：实际 fixture 的 usage 逐项正确；401 刷新重试仍只有一条逻辑请求；查询不能越权；任何记录都不含 Prompt、OAuth Token 或 Client Key。

#### Phase D：TPM 与周期 Token 硬额度

实现内容：

1. 建立 `quota_reservations/usage_buckets/usage_adjustments`。
2. 实现原子 reserve/settle/release、output `max_tokens` 预占、分钟/日/月/lifetime 桶。
3. 实现无最终 usage 的保守结算、orphan heartbeat/reaper 和手工调整审计。
4. `/limits` 返回 used/reserved/reset/enforcement_mode，明确 `reserved` 或 `postpaid`。
5. 只有接入经过验证的 Claude 兼容 tokenizer 后，才把 input/total 标为硬限制。

验收门槛：高并发预占不超卖；重复 settle 不重复扣量；DST/月末正确；账本故障时不会继续产生无法结算的上游请求。

#### Phase E：多副本与运维能力

实现内容：

1. 将 OAuth 凭据和 OAuth SessionStore 放入共享安全存储；Token 刷新使用跨副本锁加 `token_version` CAS，避免两个实例轮换 refresh token 后互相覆盖。
2. 将 RPM、并发 lease、reservation 正确性迁移到共享原子存储。
3. 加入缓存失效通知、短 TTL 兜底、reaper leader lock 和数据库连接池预算。
4. 完成备份恢复、pepper 轮换、Key 应急撤销、migration/rollback runbook。
5. 做两副本故障注入：并发 OAuth refresh、进程崩溃、Redis 抖动、DB 主从切换、网络分区和长 SSE。

验收门槛：100 个跨副本并发刷新只产生一次有效 refresh，旧 `token_version` 不能覆盖新凭据；任一副本退出不会释放其他副本的活跃 lease；同一 reservation 只回收/结算一次；陈旧策略在承诺 TTL 内失效；恢复后账本可对账。

迁移 static 到 managed 时：先建表和备份，再创建一个真实 bootstrap Client/Policy/Key，在维护窗口切换 `auth.mode`；验证后撤销旧静态 Key。不要同时开放两种认证，也不要自动把 `GATEWAY_API_KEY` 导入数据库。回滚只能回到已备份且明确启用的 static 配置，并应视为安全事件记录。

### 16.11 容易遗漏但上线前必须完善

- **管理员保护**：`admin_listen` 默认仅 loopback；生产可再加 mTLS/VPN/主机 ACL。Admin Key 也要有轮换、撤销和审计方案，不能永久共享一个无来源标识的值。
- **密钥恢复**：备份 control DB、OAuth 凭据和 pepper，但三者分开保存；恢复演练要验证权限和 `0600`。丢失 pepper 后不能解出旧 Key，只能批量撤销并重发。
- **数据库约束**：为状态枚举、非负 Token、`completed_at >= started_at`、active reservation 唯一性增加 CHECK/UNIQUE；所有外键删除策略显式定义。Client 建议禁用而非物理删除，以保留历史用量。
- **时间与窗口**：统一 UTC 存储；timezone 只用于计算业务窗口。周期上限 API 返回绝对 `resets_at`，不要让客户端猜服务器时区。
- **请求幂等**：客户端 request ID 只用于关联，不能让客户端覆盖网关生成的主键；OAuth 401 重试复用同一 request/reservation，上游 5xx 不自动重放。
- **反滥用**：认证失败限流按可信代理后的源 IP/连接维度执行，但不要把高基数 IP、Client ID 或 Key prefix 作为常驻 Prometheus label；它们只进入受控、脱敏日志。
- **策略变更**：降低额度时可能出现 `used+reserved > new_limit`，更新可以成功，但新请求立即拒绝；不能回滚或强杀已在途请求。API 应返回 `over_limit=true`。
- **Key 轮换**：需要重叠期时给旧 Key 独立 `expires_at`，不允许两个 secret 共用同一数据库 ID/hash；重叠结束由后台任务撤销并审计。
- **数据保留**：明细过期后先聚合再删除；删除任务分批执行，不能长事务锁表。保留期、审计法规和备份生命周期要一致。
- **对账修复**：提供只读 reconciliation 命令比较 usage、reservation 和 bucket；修复必须生成 `usage_adjustments`，禁止管理员直接 UPDATE 历史 Token。
- **容量预算**：按峰值 RPS 估算 usage 行增长、索引、连接池和 Redis key 数；SSE 不应每个 event 写数据库，只在状态边界和最终结算写入。
- **可观测性**：至少监控认证失败、各类拒绝、活跃 lease、reservation age、settle 失败、orphan 数、incomplete usage、DB/Redis 延迟和缓存命中；告警内容不带 Secret/Prompt。
- **启动与就绪**：migration 未完成、pepper 缺失、hard-limit store 不可用或存在无法结算的阻塞状态时 `/readyz` 应失败；`/healthz` 仍只表示进程存活。
- **协议兼容**：网关自己的限流响应必须保持 Anthropic error envelope；SSE 一旦发出响应头/首块数据，后续本地结算错误只能断流并告警，不能插入普通 JSON 错误。
- **压测与 race**：除单测外执行 `go test -race ./...`，并覆盖长 SSE、慢消费者、取消风暴、100+ 并发 reserve/settle、Key 撤销缓存竞态和优雅关机。

### 16.12 控制面测试与验收

认证授权：

- Key 创建只回显一次明文，数据库和日志不存在明文。
- 错误 Key 与正确 Key 的外部错误一致；撤销、过期、Client disabled 立即拒绝。
- Admin Key 不能当 Client Key，Client Key 不能访问 admin 路由。
- scope、原始模型、alias 后模型和 mimic 权限均无法绕过。
- Policy 乐观锁冲突返回 409；正在进行的请求使用捕获的旧快照，新请求使用新版本。

限量并发：

- RPM 边界不会在自然分钟切换时双倍突发。
- 100 个并发请求在并发上限 3 时最多 3 个持有 lease。
- JSON、SSE EOF、客户端取消、panic 和所有错误 return 都释放/续租正确。
- 限量存储故障按配置 fail closed，不会静默绕过硬限制。
- `max_tokens` 缺失、非整数、超限均按策略拒绝。

额度预占：

- 100 个并发 reservation 不会使 `used+reserved` 超过同一硬限额。
- settle 幂等，重复事件/401 重试不会重复计量。
- 上游未发出时释放；无最终 usage 的中断流按 reserved 保守结算。
- 活跃长 SSE 不会被孤儿回收器提前释放；崩溃遗留项按策略处理一次。
- 日/月窗口在 DST、月末和 timezone 变更时归属正确。

用量采集：

- JSON usage、完整 SSE、跨 chunk SSE、缺失最终 usage、429/5xx 均有 fixture。
- cache creation/read 分项和 total 公式一致。
- usage DB/队列故障不会 drop 硬额度结算。
- 自助查询只能看到认证 Client 自身，时间范围和分页有上限。
- 用量表不保存 Prompt、System、工具参数、Token 或 Client Key。

验收路径：

```text
管理员创建 Client + Policy + Key -> 明文 Key 仅显示一次
Claude Code 使用 Client Key        -> scope/model 检查后成功反代
第 4 个并发请求（limit=3）          -> 429，前三个结束后恢复
RPM/TPM/日额度达到上限              -> 429 + 合法 reset/retry 信息
SSE 正常结束                        -> usage 完整结算，reservation 清零
SSE 中途断开且无 usage              -> 保守记 reserved/incomplete，不释放成 0
撤销 Key                            -> 新请求立即 401，历史用量仍可审计
```

## 17. 核心网关测试矩阵

### 17.1 OAuth

- PKCE verifier/challenge 符合 RFC 7636。
- 授权 URL 解析后包含当前 profile 的 `code=true`、client_id、response_type、redirect_uri、scope、challenge method、challenge 和 state；不能靠参数顺序测试。
- OAuth session 30 分钟过期。
- `code#state`、完整 callback URL 和 query string 都能正确解析；裸 code、双重编码、重复参数被拒绝。
- 错误 state 被拒绝且不调用 Token 上游。
- session 只能 exchange 一次。
- Token exchange 成功后保存字段正确。
- 凭据文件权限为 `0600`。
- 原子写失败时旧凭据仍可读取。
- 日志不包含 Token 和 code。
- status 不回显 Token/账号标识且不触发刷新；logout 需要显式确认并使数据端进入 `login_required`。
- 重新授权持久化失败时旧 durable 凭据仍完整，不能先删旧值或宣告切换成功。

### 17.2 TokenProvider

- 未到刷新窗口直接返回缓存 Token。
- 到刷新窗口自动刷新。
- 100 个并发请求只触发一次刷新。
- 第一批刷新完成后，迟到的并发 401 看到 rejected token 已过时，不触发第二次刷新。
- 刷新未返回 refresh token 时保留旧值。
- 刷新返回轮换 refresh token 时替换旧值。
- 刷新成功但落盘失败进入 `credential_store_degraded`，内存新版本成为权威值，不回退或再次使用已轮换的旧 refresh token。
- degraded 后的后台任务只重试保存同一 TokenVersion，不再次调用 Token 端点；保存恢复后 readiness 自动恢复。
- 旧的后台 Save 任务不能覆盖更高 TokenVersion。
- refresh HTTP deadline 已耗尽但刷新成功时，持久化使用新的独立 Context。
- `invalid_grant` 进入 `login_required` 且停止自动刷新；网络/429/5xx 进入有界退避，不被误判为永久登出。
- 提前刷新暂时失败且旧 access token 仍有效时可以继续使用；上游 401 明确拒绝后不能复用旧 token。
- 没有凭据返回明确 `login_required`。
- 上游 401 只触发一次强制刷新和一次请求重试。

### 17.3 请求认证与校验

- Bearer 静态密钥通过。
- `x-api-key` 兼容通过。
- 缺失、错误、冲突密钥返回 401。
- 重复/多值 Authorization 或 x-api-key、畸形 Bearer 被拒绝，不会因 `Header.Get` 只取首值而绕过。
- Body 超限返回 413。
- 非法 JSON、空 model、错误 stream 类型返回 Anthropic 400。
- 顶层关键字段或 `metadata.user_id` 重复、JSON 尾随第二个值被拒绝；检测与上游看到的字段不存在解析分歧。
- 客户端认证头不会出现在上游。

### 17.4 Claude Code 透传

- 合法 Claude Code 请求被识别。
- 普通 curl UA 不被识别。
- 假 UA + 非法 metadata 不被识别。
- 官方 Claude Code Body 字节保持不变，除文档允许的定点修改。
- OAuth 身份隔离只修改 `metadata.user_id`，且同一原始 session 映射稳定。
- 进程重启后，持久化 instance/device ID 保证同一 session 的映射不变。
- metadata 被重写时，`X-Claude-Code-Session-Id` 与新 session ID 一致。
- Header 白名单正确；Cookie、Host、入口 Authorization 不透传。
- 官方客户端有入站 beta 时只补 OAuth 必需项，不注入完整模拟 profile；缺失时选择模型相关默认 profile。
- `count_tokens` 始终补 token-counting；模拟路径不混入不可信客户端 beta。
- beta 精确去重，子串不能冒充 required token；超长/非法 token 被拒绝。
- 最终 beta 缺少 context-management 时只定点删除顶层 `context_management`，其他字段不被误删。
- 入站 `anthropic-version` 合法时保留，缺失时使用配置 fallback。
- 网关不透传入站 `Accept-Encoding`；Go 自动解压后，响应不出现失效的压缩/长度头。
- 模型别名映射只改请求顶层 `model`，并在非流式/SSE 响应的协议位置精确还原客户端原模型名。
- 客户端 query string 不能更改固定上游 host/path/`beta=true`。
- 默认 Transport 不继承环境代理。

### 17.5 模型列表

- 缺失或错误的入口密钥返回 401。
- 非空列表返回 `data/has_more/first_id/last_id` Anthropic envelope，不返回 `object:list`。
- 空列表返回 `data=[]`、`has_more=false`、两个 ID 为 null。
- `/v1/models` 不触发上游调用或 Token 刷新。
- 展示列表之外的非空模型在默认策略下仍能进入 `/v1/messages` 并由上游判定。
- 未实现分页时，分页 query 参数被明确拒绝而不是静默伪造结果。

### 17.6 SSE

- 第一块上游数据到达后立即 Flush。
- 多个 SSE event 字节顺序保持。
- 大于 64 KiB 的单个 data 行不会被截断。
- SSE event 可以跨多个网络 chunk，多个 event 也可以落在同一 chunk；启用响应改写时仍能正确解析并保持事件顺序。
- 单 event 超过 `max_sse_event_bytes` 时有界失败，不会无限占用内存。
- 客户端取消会取消上游请求。
- 一个等待 Token 刷新的客户端取消，不会取消其他请求共享的刷新。
- 流开始后不进行 401/5xx 重试。
- 上游流错误不会在 SSE 后追加普通 JSON。
- `X-Accel-Buffering: no` 存在。
- SSE 不经过 gzip/brotli 中间件。
- `http.Server.WriteTimeout=0` 时长流不会被固定总时长切断。
- 下游停止读取时，每次写入的 idle deadline 能回收连接。
- 上游长时间无任何字节时触发可配置 stream idle timeout；正常 keepalive 会重置计时器。
- 优雅关机超时后会取消并回收残留 SSE。

### 17.7 可选模拟

- System 三块结构 golden test。
- billing fingerprint golden test。
- metadata legacy/JSON 编解码。
- 同一会话 session ID 稳定，不同会话不同。
- cache-control 总数不超过 4。
- 工具名在请求所有引用位置同步改写。
- 响应 JSON 和跨 chunk SSE 工具名正确恢复。
- server tools 不被改名。

## 18. 完成标准

交付物必须包含：

1. 可编译的独立网关源码。
2. 示例配置，所有密钥使用占位符。
3. OAuth 登录操作说明。
4. Claude Code 配置说明：

   ```bash
   export ANTHROPIC_BASE_URL="http://127.0.0.1:8080"
   export ANTHROPIC_AUTH_TOKEN="<GATEWAY_API_KEY>"
   ```

5. systemd 或容器部署样例，但不得自动安装或启用系统服务。
6. 单元测试、HTTP 集成测试、race test。
7. `go test ./...` 和 `go test -race ./...` 通过。
8. 一个本地 mock Anthropic 上游测试，覆盖 JSON、SSE、401 refresh、429 和大 SSE 行。
9. README 明确说明模拟模式默认关闭。
10. README 明确声明单进程/单副本边界，以及 OAuth/指纹兼容参数可能随上游变化。
11. 提供备份和重新授权流程：凭据损坏或 refresh token 失效时，网关进入 not-ready，管理员重新执行 OAuth，而不是人工编辑 Token 文件。
12. 提供兼容 profile 更新和回滚流程：OAuth Client ID、scope、CLI/Stainless 版本、beta profile、模型目录/别名作为一组版本化配置发布，并能在不泄露 Secret 的情况下确认当前生效版本。
13. 提供凭据持久化故障操作手册：`credential_store_degraded` 时不得直接重启；先恢复磁盘/权限并等待同一进程重试落盘成功，否则准备重新 OAuth。
14. 提供受管理密钥保护的登录状态和本地 logout 操作；README 明确本地清除凭据不等于 Anthropic 上游撤销。

启用 `auth.mode=managed` 时还必须交付：

15. 显式、可回滚的 control DB migration；生产启动默认不自动迁移。
16. Client/Key/Policy 管理 API、管理审计、Key/pepper 轮换和应急撤销手册。
17. scope/model/mimic 授权矩阵，以及 static/managed 双通道绕过测试。
18. RPM、并发 lease、单请求限制和明确的 fail-open/fail-closed 行为。
19. 用量账本、JSON/SSE usage fixture、自助查询隔离、保留与对账工具。
20. 若启用 Token 硬额度，交付原子 reservation/bucket、孤儿回收、幂等结算和故障注入测试。
21. `/v1/gateway/limits` 明确返回每项限制的 `limit/used/reserved/resets_at/enforcement_mode`；无法严格前置计算的 input/total 限额必须标为 `postpaid`。
22. 部署文档明确当前只支持单副本还是已完成 Phase E；单进程内存限流不得部署多个副本。

MVP 验收路径：

```text
启动网关（未登录） -> /readyz = 503
完成 OAuth 登录     -> /readyz = 200
Claude Code 发消息   -> 上游收到 OAuth Bearer + 正确 Header
非流式请求           -> Anthropic JSON 原样返回
流式请求             -> Claude Code 实时收到 SSE
Token 接近到期        -> 单次刷新后请求成功
上游 401              -> 强制刷新且最多重试一次
刷新成功但落盘失败     -> /readyz = 503，不再次刷新；修复存储后恢复
```

## 19. 当前项目核心代码索引

以下代码仅用于理解行为和提炼模块，不应整体复制。

| 主题 | 文件与核心位置 |
|---|---|
| `/v1/messages`、`count_tokens`、`models` 路由 | `backend/internal/server/routes/gateway.go:113-148` |
| 原项目完整 Handler | `backend/internal/handler/gateway_handler.go:120`（`GatewayHandler.Messages`） |
| 原项目 `/v1/models` 自定义 envelope | `backend/internal/handler/gateway_handler.go:1006-1074`（`GatewayHandler.Models`） |
| Claude Code Context 检测入口 | `backend/internal/handler/gateway_helper.go:22` |
| Claude Code 严格识别 | `backend/internal/service/claude_code_validator.go:67-197` |
| UA + metadata 快速识别 | `backend/internal/service/gateway_upstream_response.go:27-40` |
| OAuth 常量、PKCE、SessionStore | `backend/internal/pkg/oauth/oauth.go:16-205` |
| OAuth 业务编排 | `backend/internal/service/oauth_service.go:64-320` |
| OAuth 实际 HTTP 请求 | `backend/internal/repository/claude_oauth_service.go:35-279` |
| Token 缓存和刷新 | `backend/internal/service/claude_token_provider.go:54-161` |
| 刷新锁、版本与竞争恢复 | `backend/internal/service/oauth_refresh_api.go:66-223` |
| Anthropic OAuth/Setup Token 刷新器 | `backend/internal/service/token_refresher.go:35-71` |
| AES-256-GCM 参考实现 | `backend/internal/repository/aes_encryptor.go:16-95` |
| HTTP Server 长流超时设置 | `backend/internal/server/http.go:106-110` |
| 上游连接池/响应头超时 | `backend/internal/repository/http_upstream.go:1037-1076` |
| 可选 Claude/Node TLS 指纹 | `backend/internal/pkg/tlsfingerprint/dialer.go:1-115` |
| 主转发处理顺序 | `backend/internal/service/gateway_forward.go:90` |
| 上游 URL/Header 构造 | `backend/internal/service/gateway_upstream_request.go:21-210` |
| beta 合并与最终值 | `backend/internal/service/gateway_upstream_request.go:340-579` |
| beta/body 能力对称清理 | `backend/internal/service/gateway_request.go:854-915` |
| 模拟 Header | `backend/internal/service/gateway_upstream_request.go:850-877` |
| `count_tokens` | `backend/internal/service/gateway_count_tokens.go:20-150,431-594` |
| SSE 处理 | `backend/internal/service/gateway_upstream_response.go:642-1097` |
| 非流式响应 | `backend/internal/service/gateway_upstream_response.go:1331-1403` |
| 模型别名请求映射/响应还原 | `backend/internal/service/gateway_forward.go:252-290`, `backend/internal/service/gateway_upstream_response.go:915-920,1384-1387` |
| Claude Code 常量/版本/beta | `backend/internal/pkg/claude/constants.go:4-220` |
| OAuth Body 规范化 | `backend/internal/service/gateway_claude_oauth_body.go:144-320` |
| 完整模拟编排 | `backend/internal/service/gateway_claude_oauth_body.go:379-433` |
| System Prompt 三块改写 | `backend/internal/service/gateway_claude_oauth_body.go:677-941` |
| metadata 生成 | `backend/internal/service/gateway_claude_oauth_body.go:323-357,443-523` |
| metadata 格式 | `backend/internal/service/metadata_userid.go:9-103` |
| 稳定指纹 | `backend/internal/service/identity_service.go:28-208` |
| metadata 重写 | `backend/internal/service/identity_service.go:210-341` |
| billing 版本同步 | `backend/internal/service/gateway_billing_header.go:16` |
| billing fingerprint | `backend/internal/service/gateway_billing_block.go:11-95` |
| message cache 断点 | `backend/internal/service/gateway_messages_cache.go:12-160` |
| 工具名改写/恢复 | `backend/internal/service/gateway_tool_rewrite.go:15-340` |
| 原项目 API Key schema | `backend/ent/schema/api_key.go:34-147` |
| 原项目 API Key 领域模型 | `backend/internal/service/api_key.go:30-137` |
| 原项目 Key 认证中间件 | `backend/internal/server/middleware/api_key_auth.go` |
| 原项目 Key 查询 | `backend/internal/repository/api_key_repo.go:109-145` |
| 原项目 Key 生成、认证缓存、last-used、额度更新 | `backend/internal/service/api_key_service.go:263-321,583-635,788-1005` |
| 原项目 usage schema | `backend/ent/schema/usage_log.go:16-224` |
| 原项目转发后 usage 编排 | `backend/internal/service/gateway_usage_billing.go:32-180` |
| 原项目幂等 billing command | `backend/internal/service/usage_billing.go:16-172` |
| 原项目自然分钟 RPM | `backend/internal/repository/rpm_cache.go:18-129` |
| 原项目额度/RPM 前置检查 | `backend/internal/service/billing_cache_service.go:731-861` |
| 原项目并发接口和 lease 思路 | `backend/internal/service/concurrency_service.go:21-62,219-330` |

控制面参考代码不能整体复制，逐项取舍如下：

| 原项目行为 | 新网关可借鉴 | 新网关必须改掉 |
|---|---|---|
| API Key schema/domain/repository | 状态、过期、last-used 节流、认证专用窄查询 | 原项目保存/查询明文 Key；新网关只存 lookup ID + HMAC。原项目 `0=不限`；新网关 `NULL=不限、0=禁止` |
| API Key middleware/service | 认证与业务限制分层、短缓存、singleflight、撤销后失效 | 不引入 User/Group/Subscription/余额/IP ACL；不能用 `Header.Get` 静默接受重复或冲突认证头 |
| usage log / usage billing | request_id、模型映射前后字段、Token 分项、幂等 apply 思路 | 不复制美元成本、float、余额和异步 best-effort 后扣；新网关硬额度必须预占并与账本原子提交 |
| RPM cache / billing cache | Redis 服务端时间、原子操作、故障指标 | 原项目是自然分钟计数且多处 fail open；新网关用 token bucket，硬限制故障 fail closed |
| concurrency service | lease ID、TTL、heartbeat、幂等 release、启动清理 | 不复制账号池/用户排队/负载选择；Client SSE lease 必须覆盖完整流生命周期，续租丢失必须取消请求 |

特别注意：原项目是完整的多用户、分组、账号池和计费系统，其类型依赖不能带进独立网关。只提炼上述局部算法和测试思想；新网关的唯一上游主体仍是一个 Claude OAuth 凭据，Client 只是下游授权和用量隔离单位。

## 20. 参考核心代码

### 20.1 PKCE

```go
func GenerateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func GenerateCodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
```

### 20.2 Anthropic 错误格式

```go
type anthropicErrorEnvelope struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}
```

统一输出：

```json
{
  "type": "error",
  "error": {
    "type": "authentication_error|permission_error|invalid_request_error|api_error",
    "message": "..."
  }
}
```

### 20.3 安全上游请求

```go
func buildUpstreamRequest(
	ctx context.Context,
	target string,
	body []byte,
	accessToken string,
	clientHeader http.Header,
	anthropicVersion string,
	beta string,
) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	copyAllowedClaudeHeaders(req.Header, clientHeader)
	// 服务端控制字段必须在白名单复制之后最终覆盖。
	req.Header.Set("authorization", "Bearer "+accessToken)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("anthropic-beta", beta)
	return req, nil
}
```

`anthropicVersion` 必须先按第 10.1 节完成校验/fallback。`copyAllowedClaudeHeaders` 必须跳过认证、Cookie、Host、长度、`anthropic-version`、`anthropic-beta` 和 hop-by-hop Header；Authorization、版本和 beta 始终由服务端最后写入。

## 21. 给实现 AI 的最终约束

- 先完成 MVP，再实现模拟模式；不要一次性复制所有兼容逻辑。
- 以官方 Claude Code 原样透传作为默认路径。
- 所有易变指纹数据集中配置。
- 所有 Token、Cookie 和授权码默认视为秘密。
- 不使用字符串替换处理 JSON 结构；使用 `encoding/json`、`json.RawMessage` 或可靠的定点 JSON 工具。
- SSE 必须实时 Flush，流开始后绝不重试。
- 对任何自动重试给出明确次数上限。
- 测试必须验证上游永远看不到入口网关密钥。
- 模拟功能无法保证 Anthropic 永久兼容，应在 README 中明确其版本敏感性和默认关闭状态。
- 控制面严格按 Phase A-E 递进；未完成 Phase D 时不能宣称支持 Token 硬额度，未完成 Phase E 时不能宣称支持多副本。
- Claude OAuth 凭据和 Client Key 是两套完全不同的信任域；任何 Client/Admin API 都不能读取、返回或替换 Claude access/refresh token。
- 不引入余额、美元定价、订阅、终端用户注册、分组或账号池；用量与限量全部以请求数、并发数和整数 Token 表达。
- 本文标为可选的控制面只有在配置关闭时可不实现；一旦启用 managed 模式，第 16 节的认证、审计、故障和原子性约束都属于必需项，不能只建几张表后标记完成。
