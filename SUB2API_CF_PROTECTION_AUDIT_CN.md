# Sub2API × Cloudflare Pro 防 CC/DDoS 源码审计报告

> 目标域：**pigcode.ai** ｜ 目标：设计不会误伤 SDK / SSE / WebSocket / 支付 / 正常登录的 Cloudflare Pro 防护规则。
> 方法：对 Sub2API 后端源码的只读审计，每条结论附 `文件:行号`。
> 审计范围：`/home/pig/sub2api-request-audit`（分支 `request-audit`，HEAD `59f4a8917`）。
> 全程只读，未修改源码、未构建、未重启、未读取任何 `.env`/密钥。
> **对比基准缺失**：指定文档 `/home/pig/docs/CF 防护/参考－Sub2API防CC与DDoS方案.md` 不存在（详见 F 节）。

---

## 目录
- [A. 十条核心结论](#a-十条核心结论)
- [B. 完整路由矩阵](#b-完整路由矩阵)
- [C. 当前防护文档状态（第八节）](#c-当前防护文档状态第八节)
- [D. 建议 Cloudflare 规则（有序草案）](#d-建议-cloudflare-规则有序草案未执行)
- [E. 测试与回滚清单](#e-测试与回滚清单无副作用)
- [F. 无法确认的事项](#f-无法确认的事项)
- [附录：版本与构建确认（第一节）](#附录版本与构建确认第一节)

---

## A. 十条核心结论

1. **SPA 中间件在所有 API 路由之前全局注册且不校验 HTTP 方法**（`router.go:86` 的 `r.Use(frontendServer.Middleware())` 早于 `router.go:93` 的 `registerRoutes`；`embed_on.go:87-118` 只按路径前缀判断）。任何非 bypass 路径 + 非静态文件的请求，**无论方法**，都返回 `index.html` 200（`embed_on.go:103-104`），且**绕过所有 API 认证与限流**。这就是 `POST /login`、`POST /register`、`POST /任意页面` 返回 index.html 的根因。

2. **~12 个根级网关别名被 SPA 吞掉**：`shouldBypassEmbeddedFrontend`（`embed_on.go:355-370`）漏掉了 `POST /chat/completions`（`gateway.go:366`）、`POST /embeddings`（`gateway.go:373`）、`POST /messages/count_tokens`（`gateway.go:351`）、`POST /videos`（精确，`gateway.go:391`）、`/tts /stt /custom-voices /realtime /web_search`（`gateway.go:414-438`）。embed 生产镜像下这些根别名**永远到不了 handler**。含义：CF 层 block「页面路径的写方法」对真实客户端**零损失**（真实流量走 `/v1/*`）。

3. **两套限流器 Redis 失效语义相反**：`/api/v1/auth/*` 应用层限流 **fail-CLOSE**（Redis 挂→429，`rate_limiter.go:140-145`）；面板/公开限流（settings、model-plaza、admin、user）**fail-OPEN**（放行，`panel_rate_limit.go:90-94,126-129`）。且公开 IP 限流**跳过私网/代理 IP**（`panel_rate_limit.go:141-151`）——若 `trusted_proxies` 配错导致转发 IP 落私网段，公开限流形同虚设。→ **Redis 被打挂时，DB-heavy 公开口失去应用层保护，CF 边缘限流是唯一兜底。**

4. **无认证头的网关请求在中间件早期被拒，不触达 key DB**（Anthropic 式 `api_key_auth.go:94` 401 早于 `:100` 的 `GetByKey`；Google 式 `api_key_auth_google.go:51`）。但拒绝前会调用滥用计数器（`ingress_reject.go`，Redis/内存，`api_key_auth.go:37,88`）。→ 无头洪泛在应用层廉价但仍打 Redis；**在 CF 边缘 block 无认证头请求收益最大**。

5. **三个匿名可达、每请求打 DB 的放大面**：`GET /api/v1/settings/public`（每请求**无缓存**读 ~50 个设置键，`auth.go:246`→`setting_public.go:157`，SPA 渲染依赖）、`GET /api/v1/model-plaza`（`model_plaza.go:28`）、`GET /api/v1/pages/:slug/images/*filename`（**无 JWT、无限流** + DB 可见性检查 + 文件系统读，`page_handler.go:274`）。这些是主要匿名 CC 目标。

6. **所有支付 webhook 是服务器到服务器、且已在 1MB 原始 body 上做加密签名校验**（`payment_webhook_handler.go:77-83`；easypay MD5、alipay/wxpay RSA、stripe/airwallex HMAC）。→ CF 对 `/api/v1/payment/webhook/*` **必须「仅限速、绝不 challenge」**；任何 JS/交互式 challenge 都会打断回调并触发服务商重试/告警（wxpay 尤甚：未知 provider 返回 400 而非 200 ack，`payment_webhook_handler.go:93-96`）。

7. **主流量是 SDK/CLI（Claude Code、Codex、Gemini CLI）+ SSE + WebSocket，无法过人机验证**。网关路径大量 `stream=true` SSE（`gateway_handler.go:2188`）和 7 条 WebSocket 路由。→ `/v1/* /v1beta/* /backend-api/* /antigravity/*` 及根 `/responses /alpha/search` **绝不能 challenge**，只能 block/限速。

8. **Turnstile 是后台开关门控、且非全覆盖**：未开启时 `VerifyCaptcha` 直接返回 nil（`auth_service.go:451`）。主登录时序正确（captcha 在 bcrypt 前，`auth_handler.go:247` 先于 `auth_service.go:551`），但 **OAuth `bind-login` 做 DB 查询 + bcrypt 密码比对却无 captcha**（`auth_oauth_pending_flow.go:1623-1640`），仅靠 fail-close 限速。所有 OAuth **GET** start/callback/bind-start **无任何应用层限流**。

9. **静态资源查询串被源站完全忽略**（`http.FileServer`，`embed_on.go:71`），生产 embed 构建下 `/assets/*` **不存在**合法查询参数（无 `new URL(...import.meta.url)`、无 PWA/workbox、无字体 `url()` 参数、代码分割 chunk 不带 `?`）。资源头 `public, max-age=31536000, immutable` 无 ETag（`static_cache.go:13`）。→ CF 推荐**「忽略查询串作为缓存键」**（零风险，堵住 `?cachebuster=N` 缓存穿透），而非直接 block。

10. **源站请求体上限高达 256MB**（全局 `http.MaxBytesHandler`，`http.go:127-133`；网关 `max_body_size`=256MB `config.go:2361`，文本类 32MB `config.go:2362`），且无 `WriteTimeout/ReadTimeout`（`http.go:123-124`，为流式）。→ 直连源站的大 body 攻击代价低；**锁死源站只放行 CF 网段是所有规则的前提**。

---

## B. 完整路由矩阵

**全局中间件链**（`router.go:59-71`，均不认证/不限流）：`gin.New()`→`Recovery()`（`http.go:51`）→`configureTrustedProxies`（`http.go:52`）→`RequestLogger`→`SessionBindingContext`→`Logger`→`CORS`→`SecurityHeaders`→`ServerTiming`→**SPA 中间件**（`router.go:86`）→ 路由。外层还包 `http.MaxBytesHandler`(256MB)。

认证机制：**JWT**(`jwt_auth.go:32`)、**Admin JWT/x-api-key**(`admin_auth.go:28`)、**OptionalJWT**(`optional_jwt_auth.go:23`)、**Step-up 2FA**(`step_up.go:65`)、**API-Key 网关**(Anthropic 式 `api_key_auth.go`；Google 式 `api_key_auth_google.go`)。Turnstile 仅在 `auth_service.go` 内、且开关门控。

### B1. 公开/基础设施（裸引擎，SPA 已 bypass）

| 方法 | 完整路径 | 公开 | 认证 | 应用限速 | Body | 后端触达 | 文件:行 |
|---|---|---|---|---|---|---|---|
| GET | /health | 是 | 无 | 无 | — | 静态 200 | common.go:12 |
| POST | /api/event_logging/batch | 是 | 无 | **无** | 不读 | **no-op 200**（仅 1 行访问日志） | common.go:17 |
| GET | /setup/status | 是 | 无 | 无 | — | 静态 `needs_setup:false` | common.go:23 |

### B2. 认证 `/api/v1/auth/*`（组中间件：`BackendModeAuthGuard`+`auditLog`，`auth.go:29-32`；限速均 fail-close 每 IP/min）

| 方法 | 完整路径 | 公开 | Turnstile（时序） | 限速 | 后端触达 | 文件:行 |
|---|---|---|---|---|---|---|
| POST | /api/v1/auth/register | 是 | ✅ 前置（有 verify_code 时跳过） | **5** | bcrypt+DB+邮件 | auth.go:35 / handler:188 |
| POST | /api/v1/auth/login | 是 | ✅ **bcrypt 之前**（handler:247 先于 svc:551） | **20** | bcrypt+DB(+Redis TOTP) | auth.go:38 |
| POST | /api/v1/auth/login/2fa | 是 | ❌（temp-token 门控） | 20 | Redis+TOTP+DB | auth.go:41 |
| POST | /api/v1/auth/send-verify-code | 是 | ✅ 前置 | **5** | DB+**发邮件** | auth.go:50 |
| POST | /api/v1/auth/refresh | 是 | ❌ | 30 | Redis+DB | auth.go:54 |
| POST | /api/v1/auth/logout | 是 | ❌ | **无** | — | auth.go:58 |
| POST | /api/v1/auth/validate-promo-code | 是 | ❌ | 10 | DB | auth.go:60 |
| POST | /api/v1/auth/validate-invitation-code | 是 | ❌ | 10 | DB | auth.go:64 |
| POST | /api/v1/auth/forgot-password | 是 | ✅ 前置 | **5** | DB+**发邮件** | auth.go:68 |
| POST | /api/v1/auth/reset-password | 是 | ❌（reset-token 门控） | 10 | bcrypt+DB+Redis | auth.go:72 |
| GET | /api/v1/auth/oauth/{linuxdo,github,google,wechat,oidc,dingtalk}/start | 是 | ❌（仅腾讯/阿里，非 Turnstile） | **无(GET)** | 配置读+state | auth.go:75,83,94,107,181,210… |
| POST | …/start | 是 | ❌ | 20 | 同上 | auth.go:76… |
| GET | …/callback | 是 | ❌ | **无(GET)** | **第三方 token 交换+profile 拉取**+DB | auth.go:83,94,107,118,191,220 |
| GET | …/bind/start | 是 | ❌（跳过） | **无(GET)** | state | auth.go:101,112,185,214 |
| POST | …/complete-registration | 是 | ❌ | 10 | Redis+bcrypt+DB | auth.go:84,95,145,163,192,221 |
| POST | …/{bind-login} | 是 | ❌ **DB+bcrypt 无 captcha** | 10-20 | DB+bcrypt | auth_oauth_pending_flow.go:1623 |
| POST | …/{create-account} | 是 | ✅（但在 DB 查重**之后**） | 10 | DB+bcrypt | auth_oauth_pending_flow.go:1709 |
| POST | /api/v1/auth/oauth/pending/{exchange,send-verify-code} | 是 | send-code ✅ / exchange ❌ | 20/5 | Redis(+邮件) | auth.go:121,127 |
| GET/POST | /api/v1/auth/me · /revoke-all-sessions · /oauth/bind-token | 否 | — | Global(240) | JWT | auth.go:257,259,260 |

### B3. 设置/广场/页面（公开子集，SPA 已 bypass `/api/`）

| 方法 | 完整路径 | 公开 | 认证 | 限速 | 后端触达 | 文件:行 |
|---|---|---|---|---|---|---|
| GET | /api/v1/settings/public | 是 | 无 | PublicIP(300) | **每请求无缓存读 ~50 键 DB** | auth.go:246 / setting_public.go:157 |
| GET | /api/v1/settings/email-unsubscribe | 是 | `?token=` | PublicIP(300) | DB 读写 | auth.go:247 |
| GET | /api/v1/model-plaza | 条件 | OptionalJWT | PublicIP(300) | **DB**(设置+ListPlazaGroups) | model_plaza.go:28 |
| GET | /api/v1/pages/:slug/images/*filename | 是 | **无** | **无** | **DB 可见性+文件系统读** | page_handler.go:274 |
| GET | /api/v1/pages/:slug | 否 | JWT | — | — | page_handler.go:268 |
| GET | /api/v1/pages | 否 | adminAuth | — | — | page_handler.go:282 |

### B4. 管理面 `/api/v1/admin/*`（`adminAuth`+`Global(240)`+`auditLog`+`AdminComplianceGuard`，`admin.go:23-29`）
~250 条全部需 admin JWT 或 admin `x-api-key`（`admin_auth.go:28`），**无一公开**。敏感导出/备份/S3 额外需 step-up 2FA（`admin.go:399,494,593-596,623-631`）。代表：`/admin/users`(297)、`/admin/settings`(540)、`/admin/settings/panel-rate-limit`(560-561)、`/admin/ops/*`(186-272)、`/admin/accounts/*`(352-421)。

### B5. 用户面 `/api/v1/{user,keys,groups,channels,usage,announcements,redeem,subscriptions,channel-monitor*}/*`（JWT+`Global(240)`+`auditLog`，`user.go:20-26`）
全部需 JWT。`usage`(100)、api-key daily(40)、channel-monitor-v2(147) 叠加 `Heavy(60)`。`POST /api/v1/redeem` 在此组（`user.go:123`），非 `/auth`。

### B6. 支付 `/api/v1/payment/*`（`payment.go`）

| 方法 | 完整路径 | 公开 | 签名校验(原始body) | 调用方 | challenge 会否打断 | 文件:行 |
|---|---|---|---|---|---|---|
| GET+POST | /api/v1/payment/webhook/easypay | 是 | ✅ MD5(需raw) | 服务器 | **会** | payment.go:63-64 |
| POST | /api/v1/payment/webhook/alipay | 是 | ✅ RSA(需raw) | 服务器 | **会** | payment.go:65 |
| POST | /api/v1/payment/webhook/wxpay | 是 | ✅ RSA-SHA256(需raw+头) | 服务器 | **会**(400→微信重试) | payment.go:66 |
| POST | /api/v1/payment/webhook/stripe | 是 | ✅ HMAC(需raw) | 服务器 | **会** | payment.go:67 |
| POST | /api/v1/payment/webhook/airwallex | 是 | ✅ HMAC+时间戳(需raw) | 服务器 | **会** | payment.go:68 |
| POST | /api/v1/payment/public/orders/verify | 是 | 无(仅out_trade_no) | 浏览器XHR | 部分(破轮询) | payment.go:55（只读） |
| POST | /api/v1/payment/public/orders/resolve | 是 | 签名 resume_token | 浏览器XHR | 部分 | payment.go:56（可触发对账写） |
| * | /api/v1/payment/*（其余） | 否 | JWT/admin | 浏览器 | 正常策略即可 | payment.go:26-30,72-75 |

`/payment/result` 是**前端 SPA 路由**（非 Go handler），浏览器落地后调用 verify/resolve。webhook/public 组**无应用层限流**。

### B7. LLM 网关（`gateway.go`；SDK/SSE/WS 主体）
共享链：`bodyLimit(256MB)→clientRequestID→opsErrorLogger→endpointNorm→apiKeyAuth→[compositeTarget]→requireGroup*`。认证头：`Authorization: Bearer` / `x-api-key` / `x-goog-api-key`；**`?key=` 仅 `/v1beta`、`/antigravity/v1beta` 允许**（`api_key_auth_google.go:247-257`），其余 `?key=`→400（`api_key_auth.go:54`）。

| 组/路径 | 方法 | 认证 | Body | 流式 | 文件:行 |
|---|---|---|---|---|---|
| /v1/messages, /v1/responses(+/*subpath), /v1/chat/completions | POST | api-key | 256MB | **SSE** | gateway.go:187,205,224 |
| /v1/embeddings, /v1/alpha/search | POST | api-key | **32MB** | 否 | gateway.go:231,219 |
| /v1/responses (GET) | GET | api-key | — | **WebSocket** | gateway.go:220 |
| /v1/live/:call_id (GET) | GET | api-key | — | **WebSocket** | gateway.go:203 |
| /v1/realtime (GET, Grok) | GET | api-key | — | **WebSocket** | gateway.go:302 |
| /v1/messages/count_tokens, /v1/models, /v1/usage, /v1/sub2api/billing | POST/GET | api-key | 256MB | 否 | gateway.go:196,200,201,182 |
| /v1/images/*（~15 条 gen/edit/async/batches/tasks） | POST/GET/DELETE | api-key | 256MB | 否 | gateway.go:244-258 |
| /v1/videos/*（~13 条 gen/edit/ext/status/content） | POST/GET | api-key | 256MB | 否 | gateway.go:261-272 |
| /v1/{tts,stt,custom-voices,web_search}（Grok） | POST/GET/PATCH/DELETE | api-key | 256MB | 否 | gateway.go:286-317 |
| /v1beta/models, /v1beta/models/:model | GET | google(?key=) | 256MB | 否 | gateway.go:330,331 |
| /v1beta/models/*modelAction | POST | google(?key=) | 256MB | **SSE**(streamGenerateContent) | gateway.go:333 |
| **根** /responses(+/*subpath), /alpha/search, /models | POST/GET | api-key | 256/32MB | **SSE**/WS | gateway.go:344-350 |
| **根** /messages/count_tokens | POST | api-key | 256MB | 否（**被 SPA 吞**） | gateway.go:351 |
| /backend-api/codex/{responses(+/*subpath),alpha/search,models,realtime/calls,:call_id} | POST/GET | api-key | 256/32MB | **SSE**/WS | gateway.go:355-363 |
| **根** /chat/completions, /embeddings | POST | api-key | 256/32MB | **（被 SPA 吞）** | gateway.go:366,373 |
| **根** /videos,/tts,/stt,/custom-voices,/realtime,/web_search | 各 | api-key | 256MB | **（被 SPA 吞）** | gateway.go:391-438 |
| /antigravity/models | GET | api-key(**无 bodyLimit**) | — | 否 | gateway.go:448 |
| /antigravity/v1/{messages,messages/count_tokens,models,usage} | POST/GET | ForcePlatform+api-key | 256MB | **SSE** | gateway.go:460-463 |
| /antigravity/v1beta/models(+/:model,/*modelAction) | GET/POST | ForcePlatform+google | 256MB | **SSE** | gateway.go:475-477 |

**WebSocket 路由汇总**（`coderws.Accept`）：`GET /v1/responses`、`GET /responses`、`GET /backend-api/codex/responses`（`openai_gateway_handler.go:1661`）；`GET /v1/live/:call_id`、`GET /backend-api/codex/:call_id`（`openai_live.go:217`）；`GET /v1/realtime`、`GET /realtime`（`grok_audio.go:81`）。

### B8. Setup 首次安装流（独立引擎，仅安装前）
`runSetupServer()` 单独 `gin.New()`（`main.go:97-104`），仅 `setup.NeedsSetup()` 为真时挂载。`test-db`/`test-redis`/`install` 均 `setupGuard()` 门控（`setup/handler.go:54-63`），装机后一律 403 且不再注册。**装机后不可滥用**；唯一残余风险是安装前 `test-db`/`test-redis` 可拨号任意（已校验）host:port（`handler.go:169,223`）。正常运行态仅剩静态 `/setup/status`（`common.go:23`）。

---

## C. 当前防护文档状态（第八节）

**指定对比基准 `/home/pig/docs/CF 防护/参考－Sub2API防CC与DDoS方案.md` 不存在**（`/home/pig/docs/` 整个目录缺失），无法逐条比对（列入 F 节）。仓库内唯一既有防护产物是先前生成的 `CLOUDFLARE_PROTECTION_PLAN_CN.md`。对照本次源码审计，该文档的**缺口**如下（若确认另有真实文档，可针对性重做比对）：

1. **未覆盖 `POST /login` SPA 吞噬攻击面**：文档只处理 API 路径无认证头拦截，未包含「页面路径写方法 block」规则 → 攻击者可对任意页面路径 POST 得 200 index.html 绕过 API 防护。
2. **未覆盖根级别名影子**：`/chat/completions /embeddings /messages/count_tokens /videos /tts /stt /custom-voices /realtime /web_search` 根路径不在 bypass 列表，文档的 API 前缀白名单未反映此事实。
3. **静态资源随机查询参数缓存穿透**：文档未给「忽略查询串作为缓存键」规则（`?cachebuster=N` 可绕边缘缓存打源站）。
4. **未区分 fail-open/fail-close**：文档未指出 Redis 挂掉时面板/公开限流 fail-open（保护消失），因此低估了边缘限流对 `settings/public`、`model-plaza`、`pages/*/images/*` 的必要性。
5. **支付回调粒度**：文档笼统「payment skip」，未区分 webhook（服务器到服务器、绝不 challenge、仅限速）vs public/orders（浏览器 XHR、避免交互式 challenge）；且未提醒 wxpay 400-ack 会触发微信重试。
6. **可能误伤面**：文档的通用限速未排除 WebSocket 路由（`GET /v1/responses` 等）与 Gemini `?key=` 查询认证形态（`/v1beta`）、CORS `OPTIONS` 预检——这些若被无认证头规则或 challenge 命中会误杀 SDK。
7. **验收脚本缺失项**：无 `POST /login 应被 CF 403`、无「SSE/WebSocket 不被 challenge」、无「假 Key 应进应用 401 而非 CF 误杀」、无「JS 带随机查询参数被统一缓存」、无「源站不应收到被 CF 拦截的请求」等测试。

---

## D. 建议 Cloudflare 规则（有序草案，未执行）

> 顺序即优先级；`skip` 类必须最前。所有动作对 API 路径**只允许 block/rate-limit，禁止 challenge**。占位符 `$ZONE`、`pigcode.ai` 执行时注入。表达式为 CF Ruleset 语法。

### 规则 0 — 支付回调 SKIP（最前）
- **名称**：`00-skip-payment-callbacks`
- **表达式**：`http.request.uri.path matches "^/api/v1/payment/(webhook|public)/"`
- **动作**：`skip`（skip 后续 custom rules + managed challenge；**保留**限速规则 3B/5 对它计数）
- **启用**：✅ 立即。**误伤风险**：无（已服务端签名校验）。**回滚**：删除规则。**测试**：E-9/E-10。

### 规则 1 — 页面路径写方法 BLOCK（堵 POST /login SPA 吞噬）
- **名称**：`01-block-writes-to-page-paths`
- **表达式**：
  ```
  (http.request.method in {"POST" "PUT" "PATCH" "DELETE"})
  and not starts_with(http.request.uri.path, "/api/")
  and not starts_with(http.request.uri.path, "/v1/")
  and not starts_with(http.request.uri.path, "/v1beta/")
  and not starts_with(http.request.uri.path, "/backend-api/")
  and not starts_with(http.request.uri.path, "/antigravity/")
  and not starts_with(http.request.uri.path, "/responses")
  and not starts_with(http.request.uri.path, "/alpha/search")
  and not starts_with(http.request.uri.path, "/images/")
  and not starts_with(http.request.uri.path, "/videos/")
  ```
- **动作**：`block`
- **启用**：✅。**误伤风险**：低——允许列表 = SPA bypass 前缀（`embed_on.go:355-370`），真实 API 全部命中允许项；被 block 的根别名（`/chat/completions`、`/embeddings` 等）在 embed 构建下**本就返回 index.html 不可用**（结论 2）。**回滚**：改 block→log。**测试**：E-2/E-3/E-4/E-7。

### 规则 2 — 扫描器路径 BLOCK
- **名称**：`02-block-scanners`
- **表达式**：`http.request.uri.path matches "(?i)(\\.php|/wp-|/\\.env|/\\.git|/phpmyadmin|/cgi-bin/|/xmlrpc)"`
- **动作**：`block`。**启用**：✅。**误伤**：无（Go+Vue 站）。**回滚**：删除。**测试**：E-5。

### 规则 3 — 网关无认证头 BLOCK（Bearer 路径）
- **名称**：`03-block-noauth-gateway`
- **表达式**：
  ```
  http.request.method ne "OPTIONS"
  and (starts_with(http.request.uri.path, "/v1/")
       or starts_with(http.request.uri.path, "/backend-api/")
       or starts_with(http.request.uri.path, "/antigravity/v1/")
       or http.request.uri.path in {"/responses" "/alpha/search" "/models"})
  and not starts_with(http.request.uri.path, "/v1beta/")
  and not starts_with(http.request.uri.path, "/antigravity/v1beta/")
  and len(http.request.headers["authorization"]) eq 0
  and len(http.request.headers["x-api-key"]) eq 0
  and len(http.request.headers["x-goog-api-key"]) eq 0
  ```
- **动作**：`block`
- **启用**：✅。**误伤风险**：**必须排除 OPTIONS**（CORS 预检无认证头，`cors.go:90`）与 `/v1beta`、`/antigravity/v1beta`（`?key=` 查询认证，`api_key_auth_google.go:247`）。WebSocket 客户端携带 `Authorization` → 不受影响。**回滚**：block→log。**测试**：E-6/E-8。

### 规则 3B — 静态资源忽略查询串作为缓存键（Cache Rule）
- **名称**：`03b-assets-ignore-query`
- **匹配**：`starts_with(http.request.uri.path, "/assets/")`
- **动作**：Cache Rule → Cache Key → **Ignore query string**；`Edge TTL` 尊重源站 `immutable`。
- **启用**：✅（优先于「block 查询参数」）。**误伤风险**：无（源站本就忽略查询串，`embed_on.go:71`；生产资源无合法查询参数，结论 9）。**回滚**：删除 Cache Rule。**测试**：E-11/E-12。

### 规则 4 — 认证接口边缘限速
- **名称**：`04-rl-auth`
- **表达式**：`http.request.uri.path matches "^/api/v1/auth/"`
- **动作**：`block`；**speed**：单 IP `30 req / 1 min`（宽于应用层最严 5/min，仅兜 Redis fail-close 失效前的洪泛）。**启用**：✅。**误伤**：低（正常登录远低于 30/min）。**回滚**：调高阈值。**测试**：E-4。

### 规则 5 — DB-heavy 公开口边缘限速（补 fail-open 缺口）
- **名称**：`05-rl-public-db`
- **表达式**：`http.request.uri.path in {"/api/v1/settings/public"} or starts_with(http.request.uri.path,"/api/v1/model-plaza") or http.request.uri.path matches "^/api/v1/pages/[^/]+/images/"`
- **动作**：`block`；**speed**：单 IP `60 req / 1 min`。**启用**：✅（结论 3/5：应用层 fail-open + `pages/images` 无限流）。**误伤**：低。**回滚**：调高阈值。**测试**：E-14。

### 规则 6 — 模型入口边缘限速（不触碰 SSE/WS 语义）
- **名称**：`06-rl-gateway`
- **表达式**：`starts_with(http.request.uri.path,"/v1/") or starts_with(http.request.uri.path,"/backend-api/") or starts_with(http.request.uri.path,"/antigravity/") or starts_with(http.request.uri.path,"/v1beta/") or http.request.uri.path in {"/responses" "/alpha/search"}`
- **动作**：`block`；**speed**：单 IP `300 req / 1 min`（观察一周再收紧；NAT 大出口勿低于 120）。**启用**：✅。**误伤风险**：WebSocket/SSE 长连接每连接仅 1 次计数，阈值 300 安全；**动作必须是 block 而非 challenge**。**回滚**：调阈值/改 log。**测试**：E-8。

### 规则 7 — 托管 WAF 对 LLM 路径豁免（防 prompt 误报）
- **名称**：`07-skip-owasp-llm`
- **表达式**：`starts_with(http.request.uri.path,"/v1/") or starts_with(http.request.uri.path,"/v1beta/") or starts_with(http.request.uri.path,"/backend-api/") or starts_with(http.request.uri.path,"/antigravity/") or http.request.uri.path in {"/responses" "/alpha/search" "/chat/completions" "/embeddings"}`
- **动作**：Managed Rules → **skip OWASP Core Ruleset**（保留 CF Managed Ruleset）。**启用**：✅。**误伤风险**：prompt 含 SQL/`<script>` 文本会被 OWASP 误杀 403（LLM 场景必然）。**回滚**：删除 skip。**测试**：E-7。

### 规则 8 — HTML 应急 Challenge（默认关闭）
- **名称**：`08-emergency-html-challenge`
- **表达式**：`http.request.method eq "GET" and not starts_with(http.request.uri.path,"/v1") and not starts_with(http.request.uri.path,"/v1beta") and not starts_with(http.request.uri.path,"/backend-api") and not starts_with(http.request.uri.path,"/antigravity") and not starts_with(http.request.uri.path,"/api") and not starts_with(http.request.uri.path,"/responses") and not starts_with(http.request.uri.path,"/assets/") and any(http.request.headers["accept"][*] contains "text/html")`
- **动作**：`managed_challenge`。**启用**：❌ 默认禁用，被打时手动开。**误伤风险**：仅浏览器 HTML 文档，API/SDK/SSE/WS 不受影响。**回滚**：置回 disabled。**测试**：E-1（开启后浏览器仍可访问）。

### Zone 级 / 源站要点
- Zone：`ssl=strict`、`websockets=on`（结论 7 的 WS 路由必需）、`browser_check=off`、Super Bot Fight Mode 的 block/challenge **全部关闭**（会灭 SDK）。
- 源站：换 IP + 防火墙只放行 CF 网段 + 建议 Authenticated Origin Pulls（结论 10）。
- 真实 IP 链路：Caddy 配 `trusted_proxies`+`client_ip_headers CF-Connecting-IP`，sub2api 配 `server.trusted_proxies` 或开启转发信任（否则结论 3 的公开限流/风控看到的是 CF/Caddy IP）。

---

## E. 测试与回滚清单（无副作用）

| # | 测试 | 期望 | 验证的规则/结论 |
|---|---|---|---|
| E-1 | `GET https://pigcode.ai/login` | 200 + index.html（应急规则开启后浏览器仍可过） | SPA/规则8 |
| E-2 | `POST https://pigcode.ai/login` | **CF 403**（源站零日志） | 规则1/结论1 |
| E-3 | `POST https://pigcode.ai/register` | **CF 403** | 规则1 |
| E-4 | `POST https://pigcode.ai/随机页面` ×N；`/api/v1/auth/login` ×30 | 403；第 30 次后 429 | 规则1/4 |
| E-5 | `GET /wp-login.php`、`/.env` | CF 403 | 规则2 |
| E-6 | `POST /v1/messages` 无认证头 | CF block（不达源站）；带 `Authorization: Bearer sk-假` → **应用 401**（`api_key_auth.go:105`） | 规则3/结论4 |
| E-7 | `POST /v1/messages` 带真 Key + prompt 含 `SELECT * FROM users WHERE 1=1` | 200，不被 OWASP 误杀 | 规则7 |
| E-8 | `curl -N /v1/messages stream:true`（SSE）；`wss://…/v1/responses`（WS） | 逐字下发/握手成功，无 challenge、不断流 | 规则6/结论7 |
| E-9 | 各支付 webhook 模拟 POST（无有效签名） | 到达源站由应用签名校验拒绝，**非 CF 403/429/challenge** | 规则0/结论6 |
| E-10 | `POST /api/v1/payment/public/orders/verify` | 非 challenge，正常 JSON | 规则0 |
| E-11 | `GET /assets/app.js?rand=1..50` | 全部同一缓存对象（`cf-cache-status: HIT`），源站不涨请求 | 规则3B/结论9 |
| E-12 | `GET /assets/<hash>.js` 连续两次 | 第二次 `HIT` | 规则3B |
| E-13 | 直连源站 IP `https://<origin>/` | 超时/拒绝（防火墙只放行 CF） | 结论10 |
| E-14 | `GET /api/v1/settings/public` ×70/min 单 IP | 第 ~60 次后 429 | 规则5/结论5 |

**回滚总原则**：所有规则先以 `log` 动作灰度 24–48h（看 Security Events 误杀），再切 `block`/`rate-limit`；应急 challenge 规则常关；任一规则可独立删除，互不依赖（skip 类除外，删除后 OWASP 误杀风险回归，需同步评估）。

---

## F. 无法确认的事项

1. **运行镜像对应的 commit**：`llpig/sub2api:request-audit` 是**可变 tag**，每次 push `request-audit` 分支覆盖（`fork-docker-build.yml:80-99`）。源码无法回答运行态是哪个 commit——需 `docker inspect` digest、应用版本端点、或对应的不可变 `:request-audit-<shortcommit>` tag。
2. **能否构建出 `llpig/...`**：**能，当且仅当** 仓库 secret `DOCKER_HUB_USERNAME=llpig`（`fork-docker-build.yml:36`）。secret 值无法读取（且按规则禁止读取密钥）。
3. **参考文档不存在**：`/home/pig/docs/CF 防护/参考－Sub2API防CC与DDoS方案.md` 及 `/home/pig/docs/` 整个目录缺失，第八节无法对真实文档逐条比对（C 节改为对照仓库内 `CLOUDFLARE_PROTECTION_PLAN_CN.md`）。如有真实路径，可再比对。
4. **生产真实 IP 链路状态**：`server.trusted_proxies` 是否配置、`TrustForwardedIP` 开关是否开启，属运行时配置（`.env`/DB），源码不含，且禁止读取密钥/env——**未确认**。这直接决定结论 3 的公开限流是否真的生效。
5. **是否 embed 构建**：`Dockerfile:94` 用 `-tags embed`，故官方镜像应为 embed（SPA 吞噬结论成立）；但无法在不 inspect 运行镜像的前提下 100% 确认部署用的就是该 Dockerfile 产物。
6. **Turnstile 实际启用态**：`TurnstileEnabled` 是后台 DB 设置（`auth_service.go:419-451`），运行时值未确认——若关闭，则认证接口无人机验证，规则 4 边缘限速更重要。
7. **CF Pro 具体规则条数配额**：以面板实际显示为准（本草案已按小配额设计：自定义 4 条 + 限速 3 条 + Cache 1 条 + 托管 skip 1 条 + 应急 1 条）。
8. **pigcode.ai 的 DNS/源站拓扑**（是否已暴露过源站 IP、是否走 Caddy、MX 是否泄漏）：属部署事实，需确认后才能定源站锁死步骤（结论 10）。

---

## 附录：版本与构建确认（第一节）

- **分支/commit**：`request-audit`，HEAD `59f4a8917f3c79484536d0d06d7d3d6bbab83eb4`（2026-08-10 00:28:33 UTC，`fix(ci): clear response model billing checks`）。
- **Tag**：HEAD 无 tag；最近可达 `v0.1.173`；`git describe --tags` = `v0.1.173-79-g59f4a8917`（领先 79 个提交）。
- **Dockerfile**：多阶段，前端 `node:24-alpine` + 后端 `golang:1.26.5-alpine`，`-tags embed` 内嵌前端（`Dockerfile:94`），`-X main.Commit=${COMMIT}` 注入短 commit（`Dockerfile:95`）。
- **CI**：`.github/workflows/fork-docker-build.yml` —— push `request-audit` 分支构建并推送 `${DOCKER_HUB_USERNAME}/sub2api:request-audit` + `:request-audit-<shortcommit>` + `:request-audit-build-<timestamp>`（`fork-docker-build.yml:80-99`），`DOCKER_IMAGE=${{ secrets.DOCKER_HUB_USERNAME }}/sub2api`（`:36`）。
- **能否构建 `llpig/sub2api:request-audit`**：能，当且仅当 `DOCKER_HUB_USERNAME=llpig`（见 F-2）。
- **无法确认运行镜像的 commit**：`:request-audit` 可变 tag（见 F-1）。

---

*本报告为只读源码审计产物，未对服务器或 Cloudflare 做任何变更。规则草案需在灰度（log 动作）验证后再启用。*
