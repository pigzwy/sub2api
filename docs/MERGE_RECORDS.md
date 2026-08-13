# 合并记录

## 长期上游合并规则（最高优先级）

本分支是基于 `Wei-Shaw/sub2api` 的轻量二开，目标是尽量缩小与上游的长期差异，而不是同时维护两套相似功能。

后续合并上游代码时统一遵守以下规则：

1. **发现代码冲突时，默认以上游实现为准。** 如果冲突对应的本地改动不是仍然独有且明确需要保留的功能，直接用 `upstream/main` 覆盖本地实现，不继续拼接两套逻辑。
2. **发现上游已经提供相同或相似功能时，直接淘汰二开版本。** 配置、后端服务、数据库、前端页面、测试和文档应成套切换到上游实现，不能只解决文本冲突后留下半套旧代码。
3. **只有上游完全没有的独有功能才继续保留。** 如果确实必须保留，应把改动限制在最小边界，并补充独立测试，避免侵入计费、网关转发、存储等上游高频修改区域。
4. **共享文件按功能归属处理。** 同一文件同时包含上游功能和独有二开时，删除被上游接管功能的本地差异，只保留与独有二开直接相关的最小代码；不得为了保留无关二开而恢复旧版整文件。
5. **合并后必须核验差异。** 对已由上游接管的功能执行 `git diff upstream/main -- <相关文件>`，结果应为空；再运行对应后端、前端测试后完成 merge commit。

发生不确定情况时，处理优先级固定为：

```text
上游正式实现 > 上游后续安全修复 > 本地旧二开 > 历史兼容代码
```

### 当前仍需维护的独有二开（2026-08-13）

当前分支相对上游的用户可见二开主要剩余以下五组：

- 请求审计：按用户、分组和保留期记录并查询网关请求/响应。
- 请求内容拦截：命中配置规则后返回本地模拟响应，不请求上游模型。
- 管理统计自定义日期区间：订单支付概览和账号使用统计支持开始、结束日期。
- Grok 视频对象存储：完成后流式转存到独立 S3 目标，并返回预签名链接。
- `request-audit` 分支镜像构建与独立镜像标签。

响应模型计费和数据库备份/对象存储旧二开已于 2026-08-12 完整切换为上游实现，不再作为本分支独有功能维护。后续若上游接管上述剩余功能，也按本节规则删除对应二开。

## 2026-08-13：合并上游 v0.1.176

本次上游基线为 `0e82efe48`（`v0.1.176`）。

- 采用上游 Grok 4.6、JWT 订阅档位、模型级配额封禁、未知文本模型计价回退和实时用量快照实现。
- 采用上游分组逐模型定价、长上下文定价开关、渠道模型名规范化与分组平台变更后的缓存失效实现。
- 采用上游 `/x_search`、Chat/Responses 搜索兼容、Responses 能力探测和 Realtime 音频计费修复。
- 采用上游多实例定时备份 leader lock；手动备份行为不变。
- 唯一文本冲突为 `backend/cmd/server/wire_gen.go`。冲突处理同时保留上游 `channelService`、备份 leader lock，以及本分支独有的请求审计和独立视频对象存储依赖注入。
- 请求审计、请求拦截、统计日期范围和独立视频对象存储在上游仍无等价功能，因此继续保留。

本次新增数据库迁移 `221_group_model_pricing.sql`，为 `groups` 增加 `long_context_pricing_enabled` 和 `model_pricing`。迁移不删除已有业务数据。

## 2026-08-12：合并上游 v0.1.175，并由上游接管重复功能

本次合并提交为 `72caa2fc6`，上游基线为 `5935e674a`（`v0.1.175`）。

- 按实际响应模型计费已由上游正式合入，并增加确定性价格识别、冲突回退、零价保护、渠道价格边界和媒体用量保护；本地旧实现及其后续 CI 修补全部由上游版本覆盖。
- 数据库备份/对象存储已采用上游大文件分卷上传、恢复和失败清理实现；本地旧的单文件上传与失败提示二开不再保留。
- 请求审计、请求拦截和统计日期范围等上游尚未提供的二开继续保留。
- “Grok 视频完成后转存 S3”是新的独立能力，开发时复用上游对象存储配置，但不得恢复已淘汰的旧备份实现。

相关验证：

```bash
go test -p=1 -tags=unit ./internal/service -run 'ResponseModelBilling|Backup' -count=1
go test -p=1 -tags=unit ./internal/repository -run 'Backup' -count=1
pnpm --dir frontend exec vitest run \
  src/views/admin/__tests__/BackupView.spec.ts \
  src/views/admin/__tests__/UsageView.spec.ts \
  src/components/admin/usage/__tests__/UsageTable.spec.ts \
  --maxWorkers=1 --minWorkers=1
```

## 2026-07-01：二开增加订单和账号统计时间范围筛选

### 背景

管理端订单支付概览和账号使用统计原来主要按固定近 30 天范围查看数据。运营排查对账、活动周期复盘或单个账号异常用量分析时，需要按任意开始日期和结束日期重新统计金额和使用数据。

本次调整保持最小改动，只扩展已有统计接口的查询参数和管理端筛选控件，不新增数据表，不改变已有订单、账号、用户、余额或用量日志数据。

### 功能入口

- 管理端 `订单管理 -> 支付概览`：新增时间范围选择器，可选择今日、昨日、近 7 天、近 14 天、近 30 天、本月、上月或手动选择开始日期和结束日期。
- 管理端 `账号管理 -> 账号操作 -> 使用统计`：新增时间范围选择器，可按所选范围查看该账号的费用、请求数、Token、模型分布、Endpoint 分布和趋势图。

### 调整

- `GET /api/v1/admin/payment/dashboard` 增加可选 `start_date` 和 `end_date` 查询参数，格式为 `YYYY-MM-DD`。
- 支付概览接口传入 `start_date` 或 `end_date` 时，按 `paid_at >= start_date` 且 `paid_at < end_date + 1 天` 统计已支付、充值中和已完成订单。
- 支付概览保留原 `days` 参数兼容旧调用；未传自定义日期时仍按原逻辑统计固定天数。
- `GET /api/v1/admin/accounts/:id/stats` 增加可选 `start_date` 和 `end_date` 查询参数，格式为 `YYYY-MM-DD`。
- 账号统计接口传入自定义日期时，复用已有 `GetAccountUsageStats(accountID, startTime, endTime)` 统计能力，按用量日志 `created_at` 过滤。
- 前端复用现有 `DateRangePicker` 组件，不新增第三方依赖。
- 前端默认范围为当天，进入页面或打开弹窗时优先展示当日支付金额和当日账号使用数据。

### 数据影响

本次改动只增加只读统计筛选能力，不执行迁移，不新增表，不修改已有业务数据。统计结果会随选择的时间范围变化，但底层订单、账号、用户、API Key、余额和用量日志保持不变。

支付概览的待支付订单数仍统计当前全部待支付订单，不受时间范围限制。该指标表示当前待处理状态，而不是历史区间金额统计。

### 影响范围

- 影响管理端 `admin/orders/dashboard` 页面展示和对应支付概览接口。
- 影响管理端账号使用统计弹窗和对应账号统计接口。
- 不影响用户端用量统计、订单列表筛选、支付回调、余额充值、请求审计和请求内容拦截。
- 不影响 `main`、`cc`、`gy` 分支，除非后续主动合并本二开改动。

### 验证

已执行以下验证：

```bash
git diff --check
cd backend && go test ./internal/handler/admin ./internal/service -run 'Test.*(Payment|Account|Stats)' -count=1
CI=true COREPACK_ENABLE_DOWNLOAD_PROMPT=0 corepack pnpm@9.15.9 --dir frontend run build
```

前端构建存在项目既有的 Vite chunk 和动态/静态混合导入 warning，但构建已成功完成。

## 2026-06-29：request-audit 分支增加请求审计与请求拦截二开

### 背景

`request-audit` 分支用于承载请求审计和请求内容拦截这类改动较大的二开能力。该分支与 `main`、`cc`、`gy` 分支保持独立镜像标签，便于线上按功能分支部署和回滚。

请求审计用于排查异常请求。请求拦截用于在命中特定内容后直接返回本地模拟响应，避免继续请求上游模型。

### 功能入口

- 管理端 `系统设置 -> 功能开关 -> 请求审计`：开启请求和响应内容记录，配置保留时长、用户范围和分组范围。
- 管理端 `系统设置 -> 功能开关 -> 请求内容拦截`：开启本地拦截，配置生效分组和完整匹配规则。
- 管理端 `用量统计 -> 请求审计`：在请求审计开启后显示审计标签页，可查看请求体、响应体、状态码、耗时、账号、用户和 API Key。

请求内容拦截已经放在“功能开关”页面内，不再作为新的一级菜单展示。

### 调整

- 新增 `request_audit_logs` 表，记录网关请求的请求体、响应体、平台、模型、Endpoint、状态码、耗时、用户、API Key、账号、分组和 `request_id`。
- 请求体和响应体最多保存 64 KiB，避免单条日志无限增大。
- 请求审计默认关闭。范围为空表示不限用户或分组；同时配置用户范围和分组范围时取交集。
- 新增请求审计管理接口：`GET /api/v1/admin/request-audit-logs`、`GET /api/v1/admin/request-audit-logs/:id`、`POST /api/v1/admin/request-audit-logs/cleanup`。
- 新增请求内容拦截配置，支持多个分组和多条完整匹配规则。
- 请求内容拦截覆盖 OpenAI Chat Completions、Anthropic Messages 和 OpenAI Responses 入口，支持普通响应和流式响应。
- 命中请求拦截后，后端返回本地模拟响应，不再请求上游模型；如果请求审计同时开启，审计记录会标记 `is_mocked = true`。
- 请求拦截内置算术题和 Python `print(... + str(...))` 输出识别，完整匹配规则仍按配置文本精确匹配。
- GitHub Actions 增加 `request-audit` 分支镜像构建，分支推送后生成 `<DOCKER_HUB_USERNAME>/sub2api:request-audit`、`<DOCKER_HUB_USERNAME>/sub2api:request-audit-<commit>` 和 `<DOCKER_HUB_USERNAME>/sub2api:request-audit-build-<timestamp>`。

### 配置字段

| 字段 | 说明 |
|------|------|
| `request_audit_enabled` | 是否启用请求审计。默认 `false`。 |
| `request_audit_retention_hours` | 审计日志保留时长，单位小时。`0` 表示不自动清理。 |
| `request_audit_user_scope` | 请求审计用户范围，JSON 数组。空数组表示不限用户。 |
| `request_audit_group_scope` | 请求审计分组范围，JSON 数组。空数组表示不限分组。 |
| `request_intercept_enabled` | 是否启用请求内容拦截。默认 `false`。 |
| `request_intercept_rules` | 完整匹配规则，JSON 数组。每条规则包含 `match_content` 和 `response_content`。 |
| `request_intercept_keywords` | 旧版关键词字段，仅保留兼容；当前前端保存为空。 |
| `request_intercept_response` | 旧版固定响应字段，仅保留兼容；当前前端保存为空。 |
| `request_intercept_group_scope` | 请求拦截生效分组范围，JSON 数组。空数组表示不拦截任何请求。 |
| `request_intercept_group_id` | 旧版单分组字段，仅用于兼容历史配置。 |

### 部署方式

`request-audit` 分支使用独立镜像标签。Docker Hub 用户名由 GitHub Secret `DOCKER_HUB_USERNAME` 决定；当前部署使用 `llpig` 时，镜像为 `llpig/sub2api:request-audit`。

`docker-compose.yml`：

```yaml
sub2api:
  image: llpig/sub2api:request-audit
  container_name: sub2api
  restart: unless-stopped
  mem_limit: 5g
```

更新线上容器时，先确认 compose 文件中的镜像标签已经切到 `request-audit`，再拉取并重建容器：

```bash
docker compose pull sub2api
docker compose up -d sub2api
```

### 数据影响

该二开会新增 `request_audit_logs` 表和若干 `settings` 配置项，不会修改用户、分组、账号、API Key、余额或用量日志中的既有业务数据。

开启请求审计后，系统会保存请求体和响应体片段。请求内容可能包含敏感信息，生产环境应只对必要用户或分组开启，并配置保留时长或定期手动清理。

### 已知范围

- 请求拦截只覆盖 OpenAI Chat Completions、Anthropic Messages 和 OpenAI Responses 相关 HTTP 入口。
- 请求拦截不覆盖 Gemini、Images、WebSocket 或其它非上述协议入口。
- 未选择拦截分组时，即使总开关开启，也不会拦截任何请求。
- 请求审计关闭时，不会记录请求体和响应体，也不会在用量统计中显示请求审计标签页。

### 上游合并维护

上游拆分设置模块后，请求拦截配置必须在以下链路中同时保留，不能只保留网关拦截实现：

- `backend/internal/handler/admin/setting_handler.go`：管理端设置读取响应。
- `backend/internal/handler/admin/setting_handler_update.go`：更新请求、设置组装和更新响应。
- `backend/internal/service/setting_parse.go`：默认值和数据库读取解析。
- `backend/internal/service/setting_update.go`：数据库写入。

回归测试 `TestSettingHandler_UpdateSettings_RoundTripsRequestInterceptSettings` 覆盖非空规则和多分组的保存、持久化与回读；合并上游设置模块重构时必须保留该测试。

### 建议验证

上线前建议至少验证以下路径：

```bash
go test ./internal/handler -run 'TestEvaluateRequestInterceptMarksAuditMocked|Test.*Intercept|Test.*Audit' -count=1
go test ./internal/service ./internal/handler ./internal/server/routes
go test ./...
CI=true COREPACK_ENABLE_DOWNLOAD_PROMPT=0 corepack pnpm@9.15.9 --dir frontend run build
git diff --check
```

## 2026-05-10：订阅套餐额度窗口改为滚动周期

### 背景

一天订阅套餐在下午购买后，系统会在次日 0 点重置日额度。用户可以在购买当天用完一次额度，并在次日 0 点后再次获得一次额度，导致一天套餐实际获得两次日额度。

### 根因

订阅分组首次激活额度窗口和窗口重置时使用当天 0 点作为窗口开始时间。对于 14:00 购买或首次使用的订阅，日额度窗口实际从当天 00:00 开始，因此次日 00:00 就满足 24 小时重置条件。

### 调整

订阅额度窗口改为使用当前时间作为窗口开始时间：

- 首次激活窗口时，从实际激活时间开始计算。
- 自动重置窗口时，从实际重置时间开始计算。
- 管理员手动重置额度时，也从实际重置时间开始计算。

### 影响范围

该调整影响所有 `subscription_type = subscription` 的订阅分组，包括日额度、周额度和月额度窗口。普通余额分组、Simple mode、API Key 自身的 5h / 1d / 7d rate limit 不受影响。

### 验证

已新增订阅窗口单元测试，覆盖首次激活和过期日窗口重置都使用当前时间，不再回退到当天 0 点。

验证命令：

```bash
cd backend
go test ./internal/service -run 'TestCheckAndActivateWindow_UsesCurrentTime|TestCheckAndResetWindows_UsesCurrentTimeForExpiredDailyWindow|TestCalculateProgress_DailyUsage'
go test ./internal/service
```

## 2026-05-19：合并上游 v0.1.127 并完善备份失败闭环

### 背景

上游已更新到 `v0.1.127`，本地需要合并上游代码；同时系统设置里的数据库备份在上传失败时前端只显示失败，后端缺少关键阶段日志，不利于判断是网络、S3 兼容性、临时磁盘还是备份压缩链路问题。

### 根因 / 风险

备份上传原实现会把 gzip 后的备份内容整体读入内存，再调用 S3 `PutObject`。备份文件较大时会放大内存压力，且异步备份失败路径没有把 dump、upload、进度保存等阶段的失败日志打完整，用户界面也没有直接展示备份记录里的 `error_message`。

### 调整

- 合并上游最新代码，版本文件已同步到 `backend/cmd/server/VERSION = 0.1.127`。
- S3 上传改为先落临时文件再上传，保留 `ContentLength`，避免整包备份常驻内存。
- 备份后台任务补充开始、dump 失败、upload 失败、记录保存失败和完成日志。
- 管理端备份列表对失败记录展示 `error_message`，轮询失败时也使用同一失败原因提示。

### 影响范围

影响 `admin/settings` 里的数据库备份功能，重点是 PostgreSQL dump 后的 gzip 上传链路、备份任务失败记录、管理端备份列表展示。备份恢复、S3 配置保存和定时备份配置接口不改变协议。

### 验证

计划验证命令：

```bash
# 后端：需 Go 1.26.3；本机无 go 时可用 golang:1.26.3-alpine 容器执行
go test -tags=unit ./backend/internal/service -run 'TestBackupService|TestStartBackup'

# 前端
pnpm --dir frontend run typecheck
```
