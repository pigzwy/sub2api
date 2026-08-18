# 二开功能清单（request-audit 分支）

本文件是 `request-audit` 分支相对上游 `Wei-Shaw/sub2api` 的**当前二开全量清单**，按功能分组记录入口、接口、配置、文件和测试。

合并上游时的取舍规则见 [MERGE_RECORDS.md](./MERGE_RECORDS.md) 的「长期上游合并规则」；本文件只描述**现状**，历史过程记录在 MERGE_RECORDS.md。

## 基线与核对方式

| 项 | 值 |
|---|---|
| 统计日期 | 2026-08-13 |
| 上游基线 | `baeac1f3d`（`v0.1.177`，已合并入本分支） |
| 分支共同祖先 | `baeac1f3d`（`v0.1.177`） |
| 差异规模 | 129 个文件，+16296 / -163 行 |
| 其中后端 | 92 个文件，+13908 / -85 行 |
| 其中前端及其它 | 37 个文件，+2388 / -78 行 |

（前端及其它一项较早前统计明显下降，是因为根目录三份一次性中文文档已删除、上游文档的二开段落已迁入本文件。）

重新核对清单：

```bash
git fetch upstream
git diff --name-status upstream/main...request-audit   # 全部差异文件
git diff --stat upstream/main...request-audit          # 差异规模
git log --no-merges --oneline upstream/main..request-audit  # 本地提交
```

`request-audit` 当前落后上游 4 个提交（`fd82dfd52`、`e29b93a1f`、`e215c98c2`、`fbfdcef81`，均为 Grok 长上下文与媒体兜底修复），**尚未合并，也暂时不需要合并**：这几个提交在 `v0.1.176` 之后、且尚未进入任何 release（`git describe upstream/main` = `v0.1.176-5-gfbfdcef81`，`fbfdcef81` 无 tag，两边 `VERSION` 都仍是 `0.1.176`）。按惯例等上游打出下一个版本号再整体合并，避免跟随未定稿的中间状态。

## 功能一览

| 功能 | 后端 | 前端 | 迁移 | config.yaml | 系统设置项 |
|---|---|---|---|---|---|
| 请求审计日志 | ✅ | ✅ | ✅ `154` | — | 4 项 |
| 请求内容拦截 | ✅ | ✅ | — | — | 6 项 |
| Grok 媒体转存对象存储（视频 + 音频） | ✅ | ✅ | — | ✅ `video_storage` | 1 项 |
| Gemini 图像走 Images 管线 | ✅ | — | — | — | — |
| 公开设置缓存与首屏瘦身 | ✅ | ✅ | — | — | — |
| 管理统计自定义日期区间 | ✅ | ✅ | — | — | — |
| 自定义菜单打开方式 | ✅ | ✅ | — | — | 复用 `custom_menu_items` |
| 每日签到（活动） | ✅ | ✅ | ✅ `224` | — | 4 项 |
| Studio 创作台免登录接力 | — | ✅ | — | — | — |
| fork 分支镜像构建 | — | — | — | — | — |

---

## 1. 请求审计日志

按用户和分组范围记录网关请求体与响应体（各截断 64 KiB），管理后台可分页检索、查看详情并按保留时长清理。默认关闭。

**入口**

- 管理后台 `系统设置 → 功能开关 → 请求审计`：开关、保留时长、分组范围、按邮箱搜索的用户范围。
- 管理后台 `用量统计 → 请求审计`：开关打开后才出现的标签页，可查看请求体、响应体、状态码、耗时、账号、用户和 API Key。

**接口**

```text
GET  /api/v1/admin/request-audit-logs
GET  /api/v1/admin/request-audit-logs/:id
POST /api/v1/admin/request-audit-logs/cleanup?older_than_hours=
```

**系统设置项**：`request_audit_enabled`、`request_audit_retention_hours`、`request_audit_user_scope`、`request_audit_group_scope`。范围为空表示不限；同时配置用户和分组范围时取交集。

**数据库迁移**：`backend/migrations/154_request_audit_logs.sql`（新增 `request_audit_logs` 表，9 个索引）。

**新增文件**

```text
backend/ent/schema/request_audit_log.go
backend/ent/requestauditlog.go
backend/ent/requestauditlog/{requestauditlog,where}.go
backend/ent/requestauditlog_{create,delete,query,update}.go
backend/internal/repository/request_audit_log_repo.go
backend/internal/service/request_audit_log.go
backend/internal/handler/request_audit_capture.go
backend/internal/handler/admin/request_audit_handler.go
backend/migrations/154_request_audit_logs.sql
frontend/src/api/admin/requestAudit.ts
frontend/src/components/admin/usage/RequestAuditPanel.vue
```

**侵入上游的文件**

- 网关埋点：`gateway_handler.go`、`gateway_handler_chat_completions.go`、`gateway_handler_responses.go`、`openai_chat_completions.go`、`openai_gateway_handler.go`（两处 handler 构造函数新增 `requestAuditLogService`、`settingService` 参数）、`ops_error_logger.go`（在 `setOpsSelectedAccount` 回填 account_id）。
- 设置链路：`service/domain_constants.go`、`setting_parse.go`、`setting_update.go`、`setting_gateway_runtime.go`（`GetRequestAuditRuntime` 带 60 秒缓存，读取失败按关闭处理）、`settings_view.go`、`handler/dto/settings.go`、`handler/admin/setting_handler.go`、`setting_handler_update.go`。
- 装配与路由：`server/routes/admin.go`、`handler/handler.go`、`handler/wire.go`、`repository/wire.go`、`service/wire.go`、`cmd/server/wire_gen.go`。
- ent 生成代码：`ent/{client,ent,tx,mutation}.go`、`ent/hook/hook.go`、`ent/intercept/intercept.go`、`ent/predicate/predicate.go`、`ent/migrate/schema.go`、`ent/runtime/runtime.go`。
- 前端：`views/admin/UsageView.vue`、`views/admin/SettingsView.vue`、`api/admin/settings.ts`、`api/admin/index.ts`、`components/admin/usage/UsageFilters.vue`、`i18n/locales/{zh,en}/dashboard.ts`。

**测试**：`service/request_audit_log_test.go`；`server/api_contract_test.go` 与 4 个 handler 测试因构造函数签名变化被同步修改。前端无测试。

**数据风险**：审计内容可能包含敏感信息，生产只应对必要用户或分组开启，并配置保留时长。

---

## 2. 请求内容拦截

对选中分组的请求先做本地匹配，命中后直接返回本地配置的模拟响应，不请求上游模型。默认关闭；未选择分组时即使总开关打开也不拦截。

**入口**：管理后台 `系统设置 → 功能开关 → 请求内容拦截`（开关、生效分组、`match_content`/`response_content` 规则列表）。

**覆盖范围**：OpenAI Chat Completions、Anthropic Messages、OpenAI Responses 三种协议的流式与非流式。不覆盖 Gemini、Images、WebSocket 及其它入口。除精确规则外内置算术题和 Python `print(... + str(...))` 输出识别。

**系统设置项**

| 字段 | 说明 |
|---|---|
| `request_intercept_enabled` | 总开关，默认 `false` |
| `request_intercept_rules` | 规则数组，每条含 `match_content` 和 `response_content` |
| `request_intercept_group_scope` | 生效分组数组，空数组表示不拦截 |
| `request_intercept_keywords` | 旧字段，仅持久化与回显，求值逻辑不使用 |
| `request_intercept_response` | 旧字段，同上 |
| `request_intercept_group_id` | 旧单分组字段，仅兼容历史配置 |

**新增文件**

```text
backend/internal/service/request_intercept.go
backend/internal/handler/request_intercept_response.go
```

**侵入上游的文件**：与请求审计共用同一套设置链路 7 个文件；网关侧在安全审计之后插入 `if handle*RequestIntercept(...) { return }`，涉及 `gateway_handler.go`、`gateway_handler_chat_completions.go`、`gateway_handler_responses.go`、`openai_chat_completions.go`、`openai_gateway_handler.go`。上游自带的 mock 拦截路径 `sendMockInterceptStream/Response` 也加了 `setRequestAuditMocked`，与功能一联动（命中拦截时审计记录标记 `is_mocked = true`）。前端为 `views/admin/SettingsView.vue` 与 `api/admin/settings.ts`。

**测试**：`service/request_intercept_test.go`、`handler/request_intercept_response_test.go`、`handler/admin/setting_handler_auth_source_defaults_test.go`（配置往返用例）、`api_contract_test.go`。

**合并注意**：上游重构设置模块时，拦截配置必须在 `admin/setting_handler.go`、`setting_handler_update.go`、`service/setting_parse.go`、`setting_update.go` 四处同时保留，只保留网关侧实现会导致配置无法持久化。回归用例 `TestSettingHandler_UpdateSettings_RoundTripsRequestInterceptSettings` 必须保留。

---

## 3. Grok 媒体转存对象存储（视频 + 音频）

把已完成的 Grok 媒体转存到一套**独立于图片存储**的 S3 兼容桶：

- **视频**：任务完成后分片流式上传，随后状态查询返回预签名 URL，内容接口 302 重定向到已存视频，不再依赖 xAI 的短时留存。
- **音频（TTS）**：`/v1/tts` 响应写回客户端后，把整段音频异步归档到 `<audio_prefix>yyyy/mm/dd/<请求 ID>.<扩展名>`。

**入口**：管理后台 `系统设置 → 备份 → 对象存储`。上游原本的「异步生图对象存储」卡与二开的媒体卡已在前端合并成**一张卡**：顶部一套共用**凭证**，下面图片 / 视频 / 音频三个类型块，各自的开关、存储桶与 Key 前缀。保存需通过上游已有的 TOTP step-up 校验，保存后即时生效。

**共用的是凭证，不是桶**。同一个 R2/S3 账号下给图片、视频各开一个桶（`sub2-image` / `sub2-video`）是最常见的用法；早先把「存储桶」放进共用块，只要桶名不同就会判成"独立目标"并展开第二套完整凭证表单，那块虽嵌在图片里却和顶层区块长得一样，看上去像是视频的配置而音频什么都没有。改成「凭证共用 + 桶按类型」后，这种配置展开零个额外表单。

**音频没有自己的桶**：音频与视频同属 `video_storage_config`，只能有独立的 Key 前缀，卡片上以提示文案说明。

**合并只在前端，后端零改动零迁移**：图片仍然走上游的 `image_storage_config` + `ImageStorageSettingService`，视频/音频走二开的 `video_storage_config`。一次保存往两个既有接口各写一次；共用模式下把凭证字段（endpoint / region / AK / force_path_style / 新填的 Secret）复制进图片那份配置，**桶与前缀原样不动**。这样图片始终跟随上游演进，我们只维护 UI。

**图片可用另一套凭证**：加载时用 `frontend/src/views/admin/backupObjectStorage.ts` 的 `imageNeedsOwnCredentials` 判断——Secret 从不回传，只能比对可见字段，因此判定刻意保守：任何一项对不上就展开图片自己的凭证块，绝不悄悄把已有部署的图片改指到别的账号。全新安装（图片配置为空）直接跟随共用凭证。

**两种模态相互独立**：只开音频时不建视频转存，只开视频时不归档音频；S3 客户端在任一开关打开时构建。旧设置 JSON 没有 audio 字段，反序列化即为“音频关闭”，无需迁移。

**音频归档是旁路，宁丢不背压**：`AudioOffloadService.Submit` 是 fire-and-forget——响应早已写回、计费早已由 `estimateGrokVoiceAudioUsage` 落定，因此并发上限 4（队满直接丢弃）、goroutine 内 `context.WithoutCancel` + 60 秒超时 + panic recover，任何失败只落一条 `audio_task.offload_failed`（按种类每分钟限频）。S3 挂掉不会影响 TTS 响应内容、状态码或账单。

**测试连接与失败自愈**（2026-08-18 加固）：「测试连接」执行真实探测——HeadBucket 确认桶可达（拿不准时不武断，R2 的对象级令牌常拒 HeadBucket 但可写）→ 写入 `<prefix>.connection-test` → 删除，整体 8 秒超时；失败经 `classifyS3ProbeError` 归类后由接口返回稳定 `code`（`bucket_not_found` / `access_denied` / `unreachable` / `secret_unreadable` / `incomplete`），前端据此显示中英本地化提示，未知错误回退原始信息。配置解析失败**不再缓存**：下次请求自动重试（临时故障自愈，无需重启），失败日志按种类限频每分钟一条、种类变化立即放行；已存 Secret 解密失败显式报错并拒绝用密文签名（存储的 Secret 一定经 Update 加密，解不开即真故障）。

**接口**

```text
GET  /api/v1/admin/backups/video-storage      # media-storage 为同一 handler 的别名
PUT  /api/v1/admin/backups/video-storage
POST /api/v1/admin/backups/video-storage/test
```

**config.yaml 配置块**（`deploy/config.example.yaml` 末尾，`image_storage` 之后）

```yaml
video_storage:
  enabled: false                     # 默认关闭，与 image_storage 完全独立
  endpoint: ""
  region: "auto"
  bucket: ""
  access_key_id: ""
  secret_access_key: ""
  prefix: "videos/"
  force_path_style: false
  presign_expiry_hours: 24
  max_download_bytes: 536870912      # 512 MiB
  audio_enabled: false               # TTS 音频归档，独立于上面的视频开关
  audio_prefix: "audio/"
```

管理后台设置优先于配置文件。前端另有 `reuse_backup_s3` 字段，仅存于系统设置（`video_storage_config`），不在 config.yaml 中。

**新增文件**

```text
frontend/src/views/admin/backupObjectStorage.ts
backend/internal/service/video_offload.go
backend/internal/service/audio_offload.go
backend/internal/service/video_storage_settings.go
backend/internal/repository/video_offload_store.go
backend/internal/repository/video_storage_probe.go
backend/internal/config/video_storage_env_test.go
```

**侵入上游的文件**

- `service/grok_media.go`：状态计费点触发转存，内容路径优先返回已存视频，新增 `writeGrokVideoOffloadRedirect`。
- `service/openai_gateway_service.go`：新增 `videoOffload`、`audioOffload` 字段。
- `service/grok_audio.go`：`ForwardGrokVoice` 在响应写回、计费落定之后插 5 行钩子，仅 `tts` 成功响应触发归档；不碰计费与响应内容。
- `repository/image_storage_s3.go`：`S3ImageStorage` 新增 `UploadVideo`（多段上传）与 `PresignVideo`，实现 `VideoObjectStorage`。
- `service/backup_service.go`：新增 `RegisterS3ConfigInvalidator`，备份 S3 凭据变更时联动重建依赖客户端。
- `service/image_storage_settings.go`：注册失效回调，`resolveLocked` 重构。
- `handler/admin/backup_handler.go`、`server/routes/admin.go`：三个配置接口。
- 装配：`repository/wire.go`、`service/wire.go`、`cmd/server/wire_gen.go`。
- `internal/config/config.go`：新增 `VideoStorageConfig`（含 `audio_enabled` / `audio_prefix`）与对应 viper 默认值；缺默认值时同名环境变量会被静默忽略（`TestConfigKeysAreEnvReachable` 会拦下）。
- 依赖：`backend/go.mod`、`go.sum` 新增 `aws-sdk-go-v2/feature/s3/manager`。
- 前端：`api/admin/backup.ts`、`views/admin/BackupView.vue`（上游图片卡被合进同一张卡）、`i18n/locales/{zh,en}/admin/overview.ts`。
- 根 `Makefile`：`FRONTEND_CRITICAL_VITEST` 追加 `backupObjectStorage.spec.ts`（不加 CI 不会跑这个用例）。

**运行时数据**：转存记录与去重锁存在 Redis（`grok_video_offload:record:v2:`），无数据库迁移。预签名 URL 强制 https。

**测试**：`service/video_offload_test.go`、`service/video_storage_settings_test.go`（含探测分类透传、失败重试、解密失败闭合）、`service/audio_offload_test.go`（key 形态、限流丢弃、失败吞掉）、`service/video_storage_settings_audio_test.go`（新字段 round-trip、旧 JSON 兼容、音频独立开关）、`service/grok_audio_offload_test.go`（S3 故障时响应与 AudioUsage 不变）、`repository/video_offload_store_test.go`、`repository/video_storage_probe_test.go`（错误归类表测）、`config/video_storage_env_test.go`、`service/grok_media_content_test.go`；前端 `views/admin/__tests__/backupObjectStorage.spec.ts`（共用目标判定与字段复制）、`views/admin/__tests__/BackupView.spec.ts` 补了加载调用的 mock。

---

## 4. Gemini 图像走 Images 管线

让 gemini 平台分组也能使用 `/v1/images/generations` 和 `/v1/images/edits`（含异步图片任务）。内部把 OpenAI Images 请求翻译成 Gemini `generateContent`，复用原生转发路径（账号选择、并发、安全审计、按图计费），再把 `inlineData` 映射回 `{data:[{b64_json}]}`，从而自动走上游已有的 S3 转存与预签名链路。

**映射规则**

- prompt → `contents[0].parts[{text}]`；参考图（multipart 与 `data:` URL）→ `parts[{inlineData}]`。
- `size` → `generationConfig.imageConfig.imageSize`，同时是按图计费的档位来源；`aspect_ratio` 透传为 `aspectRatio`。
- `responseModalities` 固定为 `["TEXT","IMAGE"]`。
- 响应 `candidates[].content.parts[].inlineData`（兼容 `inline_data`）→ `data[{b64_json,mime_type}]`；`blockReason` 与非 `STOP` 的 `finishReason` 映射为类型化错误。
- `n` 不映射为 `candidateCount`（部分 Gemini 图像模型对 `candidateCount>1` 返回 400），按实际返回的内联图片计费。

**新增文件**

```text
backend/internal/handler/gemini_images.go
backend/internal/service/gemini_images_adapter.go
```

**侵入上游的文件**：`server/routes/gateway.go`（images 分发新增 `case service.PlatformGemini`）、`handler/image_task_handler.go`（`SetGeminiForwarder`、`supportsPlatform`、执行分支、提交时校验 model 非空）、`handler/wire.go`（`ProvideHandlers` 后置注入 forwarder，规避 Wire 循环依赖）。

**配置与迁移**：无，复用既有 `image_storage.*`。未配置对象存储时异步入口与 openai/grok 一致地整体关闭。

**测试**：`service/gemini_images_adapter_test.go`、`handler/image_task_handler_test.go`、`server/routes/gateway_test.go`。

---

## 5. 公开设置缓存与首屏瘦身

匿名 `/settings/public` 增加 5 秒进程内缓存、并发请求合并和代次防陈旧回填；新增 compact 接口把内联 Logo 与法律正文剥离为带版本号的可长缓存资源。首屏载荷按生产数据由约 145.6 KiB 降至约 5.5 KiB。

**接口**

```text
GET /api/v1/settings/public                                  # 保持完整响应，旧客户端不受影响
GET /api/v1/settings/public/compact
GET /api/v1/settings/public/logo/:revision
GET /api/v1/settings/public/legal/:revision/:document_id
```

**侵入上游的文件**

- 后端：`service/setting_public.go`（缓存、`GetCompactPublicSettings`、`GetPublicLogoAsset`、`GetPublicLoginAgreementDocument`）、`setting_service.go`（缓存字段）、`setting_update.go`、`setting_features.go`（更新后失效）、`handler/setting_handler.go`、`server/routes/auth.go`。
- 前端：`api/auth.ts`、`views/auth/LoginView.vue`、`views/auth/RegisterView.vue`、`views/public/LegalDocumentView.vue`。

**边界**：不支持安全转换的内联 Logo 格式保留原值，避免特殊格式配置后出现 404。

**测试**：`service/setting_service_public_test.go`。前端无测试。

**部署注意**：版本化路径的 Cloudflare 缓存规则只能在部署后启用；部署前缓存工具的 preflight 会因新端点尚未返回 200 而停止，不得提前改线上缓存策略。

---

## 6. 管理统计自定义日期区间

支付概览与账号使用统计支持 `start_date` / `end_date` 自定义区间，取代固定天数按钮。前端默认范围为当天。

**入口**：管理后台 `订单管理 → 支付概览`；管理后台账号管理的 `使用统计` 弹窗。

**接口**：`GET /api/v1/admin/payment/dashboard?start_date=&end_date=`、`GET /api/v1/admin/accounts/:id/stats?start_date=&end_date=`，格式 `YYYY-MM-DD`，保留原 `days` 参数兼容旧调用。支付概览按 `paid_at >= start_date` 且 `paid_at < end_date + 1 天` 统计；待支付订单数仍统计全部待支付，不受区间限制。

**侵入上游的文件**：`service/payment_stats.go`（`GetDashboardStatsByRange`）、`handler/admin/payment_handler.go`、`handler/admin/account_handler.go`；前端 `api/admin/payment.ts`、`api/admin/accounts.ts`、`views/admin/orders/AdminPaymentDashboardView.vue`、`components/admin/account/AccountStatsModal.vue`、`components/common/DateRangePicker.vue`（新增 `align` prop）。

**测试**：无。

---

## 7. 自定义菜单打开方式

自定义菜单项此前一律在站内 iframe 中打开。现在每项可单独选择打开方式，便于把新功能直接挂成一个外部标签页，而不必强行嵌进本站。

**入口**：管理后台 `系统设置 → General → 自定义菜单页面`，每个菜单项新增「打开方式」下拉框。

**三种取值**（存在菜单项 JSON 的 `open_mode` 字段里）

| 值 | 行为 |
|---|---|
| `iframe`（默认） | 站内 iframe 嵌入，与改动前完全一致 |
| `self` | 当前标签页跳转到目标 URL，离开本站 |
| `blank` | 新标签页打开目标 URL |

**兼容性**：字段可省略。改动前保存的菜单项没有 `open_mode`，解析为 `iframe`，行为不变。Markdown 页面（`md:<slug>` 或 `page_slug`）始终在站内渲染，配置界面会把该下拉框置灰。

**两个刻意的设计决定**

1. **三种模式共用 `buildEmbeddedUrl`，参数完全一致**：`user_id`、`token`、`theme`、`lang`、`ui_mode=embedded`、`src_host`、`src_url`。目标页在嵌入和跳转两种形态下拿到的地址相同，因此一个能在 iframe 里跑通的页面，换成新标签页也能直接跑通——这是本功能的实际用途（把已有工具挂成一个外部标签页）。URL 里已有的 query 会被保留，不会被覆盖。
   **代价**：跳转模式会让令牌出现在浏览器地址栏、历史记录和后续 referer 里，这是 iframe 模式没有的暴露面。接收方 origin 与 iframe 模式完全相同（都是管理员配置的那个 URL），变化的只是它对用户自己浏览器的可见性。配置界面用中英文都写明了这一点，并提示只指向自己信任的站点。若将来需要「跳转但不带令牌」，应加一个按项的开关，而不是全局改掉这里。
2. **侧边栏对非 iframe 项渲染 `<a>` 而不是 `router-link`**，让浏览器直接从用户点击完成跳转。若先路由到 `/custom/:id` 再异步 `window.open`，会被弹窗拦截器拦掉。

**兜底**：直接访问 `/custom/:id`（书签、旧链接、菜单项改过打开方式）时，`CustomPageView` 按配置的方式把目标交给浏览器；新标签页被拦截时留在原页面显示既有 iframe 视图，不会变成死路。外部模式但 URL 非 http(s) 时回落到站内路由，显示既有的「URL 未配置」空态。

**涉及文件**

- 后端：`handler/dto/settings.go`（`CustomMenuItem.OpenMode`，管理端保存走结构化 DTO，字段不加会被丢弃）、`handler/admin/setting_handler_update.go`（取值校验）。
- 前端新增：`utils/custom-menu.ts`（模式解析与外链推导）。
- 前端修改：`types/index.ts`、`components/layout/AppSidebar.vue`、`views/user/CustomPageView.vue`、`views/admin/SettingsView.vue`、`i18n/locales/{zh,en}/admin/settings.ts`。

**测试**：`frontend/src/utils/__tests__/custom-menu.spec.ts`（10 例）、`backend/internal/handler/dto/custom_menu_open_mode_test.go`（3 例，锁定字段在管理端保存与公开设置过滤两条链路上不丢失）。

**未改动**：CSP `frame-src` 仍然收录全部菜单项 URL（`GetFrameSrcOrigins`），包括已改为跳转的项。多授权一个 origin 无害，且菜单项改回 iframe 时不会因为 CSP 漏配而白屏。

---

## 8. 每日签到（活动）

把原本外挂在独立项目（pay-sub2api）里的签到功能内置进来，不再为一个签到单独跑一个容器。用户每天可签到一次，获得配置区间内的随机余额奖励。

**入口**：管理后台 `系统设置 → 功能开关 → 活动 → 每日签到`（含发放汇总）；用户端在**页面头部余额左侧**的签到徽章，悬浮展开面板，受 `checkin_enabled` 开关控制。「活动」是本次新建的分区，邀请返利在视觉上一并归入其中，后续新增活动继续追加到该分区。

**接口**

```text
GET  /api/v1/user/checkin   # 当月日历 + 累计统计 + 余额
POST /api/v1/user/checkin   # 签到；请求体仅在开启人机验证时需要
```

两个接口注册在 `routes/user.go` 的 authenticated `/user` 组内，因而自动获得 JWT 鉴权、面板限流与审计日志。功能关闭时返回 404。

**系统设置项**

| 字段 | 说明 |
|---|---|
| `checkin_enabled` | 总开关，默认 `false` |
| `checkin_min_amount` | 单次奖励下限，默认 `0.1` |
| `checkin_max_amount` | 单次奖励上限，默认 `0.3` |
| `checkin_captcha_enabled` | 签到是否要求人机验证，默认 `false` |

金额在管理端保存时夹到 `(0, 1000]` 且保证 `min <= max`；读取侧解析失败会回落到默认区间。

**数据库迁移**：`backend/migrations/224_user_checkin_records.sql`（新增 `user_checkin_records` 表）。原为 `222`，因与上游 `222_group_usage_daily_rollups.sql` 撞号，于 2026-08-15 合并 v0.1.177 时改名。

**管理端发放汇总**：设置卡片里直接显示「今日发放 / 本月发放 / 累计发放」及对应人次，接口 `GET /api/v1/admin/checkin/stats`。数字长在金额区间输入框旁边，是为了让管理员在决定要不要调区间时不必换页面查账。该接口**在功能关闭时照常返回历史数据**——关掉开关不该让已发生的支出从管理端消失。统计走一条带 `FILTER` 的聚合查询，三个口径取自同一时间点；明细可在余额变动记录里按 `checkin` 类型筛选。

**三个关键设计**

1. **判重靠数据库唯一索引，不靠先查后写**。`UNIQUE(user_id, checkin_date)` 是唯一权威判重点：并发双击时两个请求都会尝试插入，只有插入成功的那一个才继续加余额，另一个拿到唯一约束冲突并被翻译成 `CHECKIN_ALREADY_DONE`。
2. **签到记录、加余额、余额流水三者同事务**。参考项目里踩过的坑是「入账失败却把这天标记成已签到」，用户既没拿到钱也不能重试。这里三步在同一个 ent 事务内完成，任一失败整体回滚，于是「今天已签」「余额已到账」「余额记录里查得到」三者永远一致。
   余额流水写进 `redeem_codes`（类型 `checkin`），因为管理端「余额变动记录」正是由该表与邀请返利流水归并而成，管理员手动充值也是这么落记录的。类型独立于 `balance`/`admin_balance`，因此 `SumPositiveBalanceByUser` 不会把签到算进「累计充值」——与第 3 条同一个考量。
   流水插入走仓储内的原生 SQL 而非 `redeemCodeRepo.Create`：后者用构造时注入的 client、不认事务上下文，在事务里调用会让记录游离于事务之外，回滚时余额没加而记录留下，恰好破坏这里要保证的闭环。
3. **奖励用 `crypto/rand` 而非 `math/rand`**。金额直接变成余额，可预测的随机序列意味着可以挑时机签到。取样以「分」为单位取整数随机数，避免浮点取样后四舍五入越界。

**不新建 Ent schema**：`user_checkin_records` 与 `user_affiliates` 一样是纯迁移 + 裸 SQL 仓储，通过项目已启用的 `sql/execquery` feature 在 ent 事务内执行原生 SQL。这样省掉一次 `go generate ./ent` 带来的大量生成代码 diff。

**验证码**：走 `AuthService.VerifyCaptcha` 而不是 `VerifyActionCaptchaIfEnabled`——后者只认腾讯/阿里，站点只配了 Turnstile 时会静默放行，那样这个开关就名不副实。前端验证码组件只在用户点击签到后才挂载，避免每个打开页面的人都被挑战一次。

**涉及文件**

- 后端新增：`migrations/224_user_checkin_records.sql`、`service/checkin_service.go`、`repository/checkin_repo.go`、`handler/checkin_handler.go`。
- 后端修改（设置链路，与既有开关同一套路）：`service/domain_constants.go`、`setting_parse.go`、`setting_update.go`、`setting_public.go`、`setting_features.go`、`settings_view.go`、`handler/dto/settings.go`、`handler/setting_handler.go`、`handler/admin/setting_handler.go`、`setting_handler_update.go`、`setting_handler_audit.go`；装配 `repository/wire.go`、`service/wire.go`、`handler/wire.go`、`handler/handler.go`、`cmd/server/wire_gen.go`、`server/routes/user.go`。
- 前端新增：`components/layout/HeaderCheckin.vue`（签到入口后来从独立页面改成顶栏面板，原 `views/user/CheckinView.vue` 已删除）。
- 前端修改：`components/layout/AppHeader.vue`（挂载签到面板，签到后刷新余额）、`components/admin/user/UserBalanceHistoryModal.vue` 与 `views/user/RedeemView.vue`（余额流水识别 `checkin` 类型；签到发放的兑换码是内部随机串，不向用户展示）、`types/index.ts`、`stores/app.ts`、`utils/featureFlags.ts`、`api/user.ts`、`api/admin/settings.ts`、`router/index.ts`、`components/layout/AppSidebar.vue`、`views/admin/SettingsView.vue`、`i18n/locales/{zh,en}/{common,dashboard,admin/settings}.ts`。

**上游 PR 状态（2026-08-15）**：已按 [MERGE_RECORDS.md](./MERGE_RECORDS.md) 的「向上游提 PR 的基线校准」流程，从上游 `c204d33b0` 切出只含签到的分支 `feat/daily-checkin`（已推送 `origin`，**尚未提 PR**，先自用观察）。本分支的迁移已改名为 `224_user_checkin_records.sql`（原 `222` 与上游 `222_group_usage_daily_rollups.sql` 撞号）。上游 v0.1.178 又新增了 `224_user_platform_quotas_add_cn_providers.sql`，同号不冲突——`migrations_runner.go` 的 `schema_migrations` 以**文件名**为键，上游自身也有三个 `028_*`，两条改的又是不同的表。真正提 PR 前应重新 rebase 到当时的 `upstream/main` 并完整重跑验证。

**测试**：`service/checkin_service_test.go`（随机金额区间/取整/退化区间/两端可达、按用户按日期判重、流水写入失败时三者一并回滚、成功路径三者齐落）；`server/api_contract_test.go` 的设置契约快照同步了新字段。

---

## 9. fork 分支镜像构建

`.github/workflows/fork-docker-build.yml`（`[FORK] Build & Push Docker Image`）是唯一的 CI 二开，上游 workflow 均未改动。

- 触发：push 到 `main` / `cc` / `gy` / `request-audit`，或手动触发并可指定 tag。其中 `cc` 和 `gy` 在 `origin` 上已不存在，触发条件形同虚设，可在下次改动该 workflow 时一并删除。
- 镜像：`${DOCKER_HUB_USERNAME}/sub2api`，标签 `main → latest`、`request-audit → request-audit`；每次推送三个标签 `:<branch>`、`:<branch>-<short_sha>`、`:<branch>-build-<UTC 时间戳>`。
- 版本号优先取 `git describe --tags --match 'v*'`，回落读 `backend/cmd/server/VERSION`。

当前 Docker Hub 用户名为 `llpig` 时，本分支镜像为 `llpig/sub2api:request-audit`：

```yaml
sub2api:
  image: llpig/sub2api:request-audit
  container_name: sub2api
  restart: unless-stopped
  mem_limit: 5g
```

```bash
docker compose pull sub2api
docker compose up -d sub2api
```

### 各分支镜像标签对照

| 分支 | 镜像标签 | 说明 |
|------|----------|------|
| `main` | `<DOCKER_HUB_USERNAME>/sub2api:latest` | 主线同步上游后的默认部署镜像 |
| `cc` | `<DOCKER_HUB_USERNAME>/sub2api:cc` | 分支在 `origin` 上已不存在，标签形同虚设 |
| `gy` | `<DOCKER_HUB_USERNAME>/sub2api:gy` | 同上 |
| `request-audit` | `<DOCKER_HUB_USERNAME>/sub2api:request-audit` | 本分支部署镜像 |

切换镜像标签时，先确认 `docker-compose.yml` 里的 `image` 已改好，再执行上面的 pull 与 up。

---

## 10. Studio 创作台免登录接力

公开路由 `/connect/studio`，把本站会话接力给外部的 Studio（创作台）站点，实现「已登录 sub2 即秒进创作台」。

**行为**：已登录时带当前 access token 跳到 `https://chat.pigcode.ai/studio?token=<token>`（Studio 侧的 `?token=` 入口会落盘并复验）；未登录时带 `?sso=miss` 弹回 Studio 自己的账号密码登录页，**不劫持到本站登录页**。网关菜单 iframe、独立 tab、官网首页入口统一指向本路由；Studio 侧无会话时也会跳来探测。

**为什么是公开路由**：挂 `requiresAuth` 会让未登录用户被本站登录页拦住，破坏「弹回 Studio 登录页」的语义，所以 `meta.requiresAuth: false`，由页面自己判断有没有 token。token 从 `authStore.token` 取，store 尚未初始化时回落读 `localStorage` 的 `auth_token`。

**注意**：`STUDIO_URL` 是写死的常量，换域名要改代码；token 通过 URL query 传递，会进入浏览器历史与 Studio 侧的访问日志。

**涉及文件**：前端新增 `views/ConnectStudioView.vue`；修改 `router/index.ts`（新增 `ConnectStudio` 路由）。无后端改动、无迁移、无设置项。

---

## 媒体转存与异步图片对象存储的补充说明

上游 `docs/ASYNC_IMAGE_TASKS.md` 描述的是异步图片任务与 `image_storage`。本分支在其基础上增加了**独立的媒体对象存储**（视频 + TTS 音频），与图片存储互不影响：

- 要接入 S3 之外的图片厂商，实现 `service.ImageStorage`（`Save(ctx, key, contentType, data) (url, error)`）即可；**视频转存另需实现 `service.VideoObjectStorage`**（`UploadVideo`，接收 `io.Reader`；外加 `PresignVideo`），**音频归档需实现 `service.AudioObjectStorage`**（方法集与 `ImageStorage` 相同，故 `*S3ImageStorage` 天然满足）。
- 已完成的 Grok 视频使用独立的 `video_storage` 开关与 S3 目标，配置见本文件第 3 节。
- 音频触发时机：`/v1/tts` 上游返回 <400 时，响应写回客户端后异步归档；`/stt`、`custom-voices`、realtime 一律不归档。
- 视频触发时机：xAI 侧完成状态值为 `done`，第一次成功的 `GET /v1/videos/generations/{request_id}` 状态轮询会把上游视频以分片方式流式写入 `<prefix>` 下，然后在完成 JSON 里补上新的 `video_url` 与 `url_expires_at`。内容接口在该记录存在时重定向到新签发的预签名 URL；上传失败则保持原有的状态与内容透传行为不变。
- 异步图片任务的平台支持范围本分支已扩展：除 OpenAI 与 Grok 外，**gemini 分组也可使用**（见本文件第 4 节）。上游文档中「Only OpenAI and Grok groups are supported」的表述对本分支不适用。

---

## 非功能性差异

以下差异不属于用户可见功能，但也在 diff 中：

- **文档**：二开文档只有 `docs/MERGE_RECORDS.md` 与 `docs/FORK_FEATURES.md`（本文件）两份。**所有上游文档保持与上游逐字一致，零本地改动**——原先加在 `DEV_GUIDE.md` 与 `docs/ASYNC_IMAGE_TASKS.md` 里的二开章节已于 2026-08-13 全部迁入本文件，以消除这两处合并冲突面。根目录曾有三份一次性产出的中文文档（Claude Code OAuth 独立网关规格、Cloudflare 防护方案与源码审计），同日删除，需要时从 git 历史取回，见 [MERGE_RECORDS.md](./MERGE_RECORDS.md) 对应条目。
- **订阅每日窗口测试残留**：`service/subscription_window_test.go`（本地新增）、`subscription_assign_idempotency_test.go`、`user_subscription_daily_quota_test.go` 的 stub 起点调整，以及 `subscription_service.go` 的一行注释翻译。**业务逻辑与上游完全一致**，属于历史二开被上游取代后剩下的测试侧残留，可在下次合并时考虑清理。
- **`frontend/package.json`**：`pnpm.overrides` 比上游多一条 `nanoid@<3.3.18` 安全下限约束。不影响功能，但会让 `pnpm-lock.yaml` 与上游长期不同。
- **根 `Makefile`**：`FRONTEND_CRITICAL_VITEST` 追加了 `backupObjectStorage.spec.ts`。CI 的 frontend job 只跑这个白名单，不登记等于用例不会被执行。
- **`paseo.json`**：内容为 `{}` 的工具占位文件。

## 上游合并冲突高发点

按侵入程度排序，合并上游时优先检查：

1. **装配文件**：`cmd/server/wire_gen.go`、`service/wire.go`、`repository/wire.go`、`handler/wire.go`、`server/routes/admin.go` —— 被 4 组功能反复修改。
2. **设置链路 7 文件**：`service/domain_constants.go`、`setting_parse.go`、`setting_update.go`、`settings_view.go`、`handler/dto/settings.go`、`handler/admin/setting_handler.go`、`setting_handler_update.go` —— 同时承载请求审计、请求拦截，以及自定义菜单打开方式（`dto/settings.go` 的 `CustomMenuItem.OpenMode` 与 `setting_handler_update.go` 的取值校验）。
3. **网关热路径**：`gateway_handler.go`、`gateway_handler_chat_completions.go`、`gateway_handler_responses.go`、`openai_chat_completions.go`、`openai_gateway_handler.go` —— 审计埋点与拦截判断都插在这里。
4. **前端大文件**：`views/admin/SettingsView.vue`（+487 行，承载审计、拦截两组配置与自定义菜单打开方式下拉框）、`views/admin/BackupView.vue`（+145）、`views/admin/UsageView.vue`（+41）。
5. **导航与自定义页面**：`components/layout/AppSidebar.vue`（+65/-22）、`views/user/CustomPageView.vue`（+41）—— 自定义菜单打开方式引入。**这两个是上游高频改动的布局文件，且本地改动是「改写既有结构」而非「追加新块」**，冲突概率高于上面几处按块追加的改动：
   - `AppSidebar.vue`：三处 `router-link` 被改成 `<component :is="item.href ? 'a' : RouterLink">`。上游若重构侧边栏渲染或调整这几处链接属性，会直接冲突。合并时保留「外链项渲染成 `<a>`」这一语义即可，标记类名与 `data-tour` 等属性以上游为准。另注意 `NavItem` 接口新增了 `href`/`newTab` 两个可选字段，以及 `customMenuNavItem` / `navLinkProps` 两个本地函数。
   - `CustomPageView.vue`：新增 `externalUrl` 计算属性与 `followExternalTarget`，以及一个 `watch(externalUrl)`。该文件其余部分（Markdown 渲染、TOC、iframe 嵌入）均为上游实现，合并时应整体采用上游版本后再把这三块搬回。

**Go 版本升级必查 Dockerfile**：上游 v0.1.177 把 `backend/go.mod` 升到 1.26.6、四处 workflow 断言也同步改了，**但漏了三个 Dockerfile**（根 `Dockerfile`、`deploy/Dockerfile`、`backend/Dockerfile`），它们仍钉在 `golang:1.26.5-alpine`。官方 golang 镜像设了 `GOTOOLCHAIN=local`，容器里的 Go 低于 go.mod 要求时不会自动下载工具链，直接报 `go.mod requires go >= 1.26.6 (running go 1.26.5; GOTOOLCHAIN=local)`，镜像构建在 `go mod download` 一步失败。本分支已在 2026-08-15 修正为 1.26.6。
注意上游 `DEV_GUIDE.md` 的升级指引只列了 go.mod 与四处 workflow 断言，**没有提 Dockerfile**——这正是它自己翻车的原因。合并任何带 Go 版本变更的上游代码时，务必执行：

```bash
grep -rn "golang:1\.[0-9.]*-alpine" Dockerfile deploy/Dockerfile backend/Dockerfile
grep "^go " backend/go.mod
```

两者必须一致。

**文档不再是冲突点**：所有上游文档（`README*.md`、`CLA.md`、`DEV_GUIDE.md`、`docs/` 下 7 份）本地零改动，可在合并时直接采用上游版本，无需人工比对。二开说明一律写在本文件里。这条约束请在后续改动中保持——需要补充上游功能的二开行为时，写进本文件对应章节并在其中指明上游文档的哪一句不适用，而不是回头去改上游文档。

## 已由上游接管、不再维护的历史二开

再次遇到相关需求时应直接使用上游实现，不要恢复旧代码：

| 历史二开 | 接管时间 | 说明 |
|---|---|---|
| 按响应模型计费 | 2026-08-12 | 上游实现含确定性价格识别、冲突回退、零价保护、渠道价格边界与媒体用量保护，本地实现已完全移除，diff 中无残留 |
| 数据库备份与对象存储 | 2026-08-12 | 上游实现含大文件分卷上传、恢复与失败清理；当前 `backup_service.go` 的本地差异只是视频转存所需的 `RegisterS3ConfigInvalidator` 钩子 |
| 订阅额度滚动窗口 | 上游自行实现 | 上游改为按自然日对齐的 `automaticDailyWindowStartAt`，生产代码与上游一致，仅剩测试侧残留 |

## 验证命令

```bash
# 后端
cd backend
go build ./...
go test -tags unit ./internal/service/ ./internal/handler/ ./internal/server/routes/ ./internal/repository/
go test ./...

# 前端
pnpm --dir frontend run build
pnpm --dir frontend exec vitest run src/views/admin/__tests__/BackupView.spec.ts
```

2026-08-13 在上游基线 `fbfdcef81` 上的实测结果：`go build ./...` 通过；`service`、`handler`、`server/routes`、`repository` 四个包的 unit 标签测试全部通过。
