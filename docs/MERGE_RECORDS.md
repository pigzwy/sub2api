# 合并记录

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
