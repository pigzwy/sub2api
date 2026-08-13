# 二开功能清单（request-audit 分支）

本文件是 `request-audit` 分支相对上游 `Wei-Shaw/sub2api` 的**当前二开全量清单**，按功能分组记录入口、接口、配置、文件和测试。

合并上游时的取舍规则见 [MERGE_RECORDS.md](./MERGE_RECORDS.md) 的「长期上游合并规则」；本文件只描述**现状**，历史过程记录在 MERGE_RECORDS.md。

## 基线与核对方式

| 项 | 值 |
|---|---|
| 统计日期 | 2026-08-13 |
| 上游基线 | `fbfdcef81`（`main` 分支已同步到此提交） |
| 分支共同祖先 | `0e82efe48`（`v0.1.176`） |
| 差异规模 | 123 个文件，+18649 / -137 行 |
| 其中后端 | 91 个文件，+13843 / -85 行 |
| 其中前端及其它 | 32 个文件，+4868 / -52 行 |

重新核对清单：

```bash
git fetch upstream
git diff --name-status upstream/main...request-audit   # 全部差异文件
git diff --stat upstream/main...request-audit          # 差异规模
git log --no-merges --oneline upstream/main..request-audit  # 本地提交
```

`request-audit` 当前落后上游 4 个提交（`fd82dfd52`、`e29b93a1f`、`e215c98c2`、`fbfdcef81`，均为 Grok 长上下文与媒体兜底修复），尚未合并。

## 功能一览

| 功能 | 后端 | 前端 | 迁移 | config.yaml | 系统设置项 |
|---|---|---|---|---|---|
| 请求审计日志 | ✅ | ✅ | ✅ `154` | — | 4 项 |
| 请求内容拦截 | ✅ | ✅ | — | — | 6 项 |
| Grok 视频转存对象存储 | ✅ | ✅ | — | ✅ `video_storage` | 1 项 |
| Gemini 图像走 Images 管线 | ✅ | — | — | — | — |
| 公开设置缓存与首屏瘦身 | ✅ | ✅ | — | — | — |
| 管理统计自定义日期区间 | ✅ | ✅ | — | — | — |
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

## 3. Grok 视频转存对象存储

Grok 视频任务完成后，把视频分片流式上传到一套**独立于图片存储**的 S3 兼容桶；随后状态查询返回预签名 URL，内容接口 302 重定向到已存视频，不再依赖 xAI 的短时留存。

**入口**：管理后台 `系统设置 → 备份 → 异步视频对象存储`。可勾选复用备份 S3 凭据，也可指向完全独立的账号。保存需通过上游已有的 TOTP step-up 校验，保存后即时生效。

**接口**

```text
GET  /api/v1/admin/backups/video-storage
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
```

管理后台设置优先于配置文件。前端另有 `reuse_backup_s3` 字段，仅存于系统设置（`video_storage_config`），不在 config.yaml 中。

**新增文件**

```text
backend/internal/service/video_offload.go
backend/internal/service/video_storage_settings.go
backend/internal/repository/video_offload_store.go
backend/internal/config/video_storage_env_test.go
```

**侵入上游的文件**

- `service/grok_media.go`：状态计费点触发转存，内容路径优先返回已存视频，新增 `writeGrokVideoOffloadRedirect`。
- `service/openai_gateway_service.go`：新增 `videoOffload` 字段。
- `repository/image_storage_s3.go`：`S3ImageStorage` 新增 `UploadVideo`（多段上传）与 `PresignVideo`，实现 `VideoObjectStorage`。
- `service/backup_service.go`：新增 `RegisterS3ConfigInvalidator`，备份 S3 凭据变更时联动重建依赖客户端。
- `service/image_storage_settings.go`：注册失效回调，`resolveLocked` 重构。
- `handler/admin/backup_handler.go`、`server/routes/admin.go`：三个配置接口。
- 装配：`repository/wire.go`、`service/wire.go`、`cmd/server/wire_gen.go`。
- 依赖：`backend/go.mod`、`go.sum` 新增 `aws-sdk-go-v2/feature/s3/manager`。
- 前端：`api/admin/backup.ts`、`views/admin/BackupView.vue`、`i18n/locales/{zh,en}/admin/overview.ts`。

**运行时数据**：转存记录与去重锁存在 Redis（`grok_video_offload:record:v2:`），无数据库迁移。预签名 URL 强制 https。

**测试**：`service/video_offload_test.go`、`service/video_storage_settings_test.go`、`repository/video_offload_store_test.go`、`config/video_storage_env_test.go`、`service/grok_media_content_test.go`；前端 `views/admin/__tests__/BackupView.spec.ts` 补了加载调用的 mock。

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

## 7. fork 分支镜像构建

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

## 视频转存与异步图片对象存储的补充说明

上游 `docs/ASYNC_IMAGE_TASKS.md` 描述的是异步图片任务与 `image_storage`。本分支在其基础上增加了**独立的视频对象存储**，两者互不影响：

- 要接入 S3 之外的图片厂商，实现 `service.ImageStorage`（`Save(ctx, key, contentType, data) (url, error)`）即可；**视频转存另需实现 `service.VideoObjectStorage`**（`UploadVideo`，接收 `io.Reader`；外加 `PresignVideo`）。
- 已完成的 Grok 视频使用独立的 `video_storage` 开关与 S3 目标，配置见本文件第 3 节。
- 触发时机：xAI 侧完成状态值为 `done`，第一次成功的 `GET /v1/videos/generations/{request_id}` 状态轮询会把上游视频以分片方式流式写入 `<prefix>` 下，然后在完成 JSON 里补上新的 `video_url` 与 `url_expires_at`。内容接口在该记录存在时重定向到新签发的预签名 URL；上传失败则保持原有的状态与内容透传行为不变。
- 异步图片任务的平台支持范围本分支已扩展：除 OpenAI 与 Grok 外，**gemini 分组也可使用**（见本文件第 4 节）。上游文档中「Only OpenAI and Grok groups are supported」的表述对本分支不适用。

---

## 非功能性差异

以下差异不属于用户可见功能，但也在 diff 中：

- **文档**：二开文档只有 `docs/MERGE_RECORDS.md` 与 `docs/FORK_FEATURES.md`（本文件）两份。**所有上游文档保持与上游逐字一致，零本地改动**——原先加在 `DEV_GUIDE.md` 与 `docs/ASYNC_IMAGE_TASKS.md` 里的二开章节已于 2026-08-13 全部迁入本文件，以消除这两处合并冲突面。根目录曾有三份一次性产出的中文文档（Claude Code OAuth 独立网关规格、Cloudflare 防护方案与源码审计），同日删除，需要时从 git 历史取回，见 [MERGE_RECORDS.md](./MERGE_RECORDS.md) 对应条目。
- **订阅每日窗口测试残留**：`service/subscription_window_test.go`（本地新增）、`subscription_assign_idempotency_test.go`、`user_subscription_daily_quota_test.go` 的 stub 起点调整，以及 `subscription_service.go` 的一行注释翻译。**业务逻辑与上游完全一致**，属于历史二开被上游取代后剩下的测试侧残留，可在下次合并时考虑清理。
- **`paseo.json`**：内容为 `{}` 的工具占位文件。

## 上游合并冲突高发点

按侵入程度排序，合并上游时优先检查：

1. **装配文件**：`cmd/server/wire_gen.go`、`service/wire.go`、`repository/wire.go`、`handler/wire.go`、`server/routes/admin.go` —— 被 4 组功能反复修改。
2. **设置链路 7 文件**：`service/domain_constants.go`、`setting_parse.go`、`setting_update.go`、`settings_view.go`、`handler/dto/settings.go`、`handler/admin/setting_handler.go`、`setting_handler_update.go` —— 同时承载请求审计与请求拦截。
3. **网关热路径**：`gateway_handler.go`、`gateway_handler_chat_completions.go`、`gateway_handler_responses.go`、`openai_chat_completions.go`、`openai_gateway_handler.go` —— 审计埋点与拦截判断都插在这里。
4. **前端大文件**：`views/admin/SettingsView.vue`（+447 行，承载审计与拦截两组配置）、`views/admin/BackupView.vue`（+145）、`views/admin/UsageView.vue`（+41）。

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
