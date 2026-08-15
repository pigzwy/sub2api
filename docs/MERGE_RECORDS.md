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

### 当前仍需维护的独有二开（2026-08-15 更新）

完整清单见 [FORK_FEATURES.md](./FORK_FEATURES.md)，含每项功能的入口、接口、配置、文件清单、测试和合并冲突高发点。概览：

- 请求审计：按用户、分组和保留期记录并查询网关请求/响应。
- 请求内容拦截：命中配置规则后返回本地模拟响应，不请求上游模型。
- 管理统计自定义日期区间：订单支付概览和账号使用统计支持开始、结束日期。
- Grok 视频对象存储：完成后流式转存到独立 S3 目标，并返回预签名链接。
- 公开设置性能优化：进程内短缓存、精简首屏载荷、版本化 Logo 和法律正文按需加载。
- Gemini 图片兼容：Gemini 图片模型可复用 OpenAI Images/异步任务入口，并接入现有对象存储转存链路。
- 自定义菜单打开方式：每个菜单项可选 iframe 嵌入、当前标签页跳转或新标签页打开。
- 每日签到（活动）：每天一次随机余额奖励，金额区间与人机验证可配；已另切 `feat/daily-checkin` 作为上游 PR 分支。
- `request-audit` 分支镜像构建与独立镜像标签。

已由上游接管、不再作为本分支二开维护的功能：按响应模型计费、数据库备份与对象存储（均于 2026-08-12 切换）、订阅额度滚动窗口（上游自行实现按自然日对齐的版本）。它们的历史记录已从本文件删除，避免与现状冲突；再次遇到相关需求时直接使用上游实现。后续若上游接管上述剩余功能，也按本节规则删除对应二开与记录。

## 向上游提 PR 的基线校准（提 PR 前必读）

上游作者反馈：上一次 PR 因为**没有校准基线**，合并起来很费劲。本节是为避免重演而定的固定流程。

根因是本分支的功能提交长在 fork 的历史上，而 fork 的基线往往落后上游若干提交，且相邻代码里混着别的二开。直接把功能分支或 cherry-pick 结果推给上游，会同时带上三类污染：过期的编号与行号、别的二开代码、以及含 fork 依赖的生成文件。

### 固定流程

```bash
git fetch upstream
git branch feat/<功能名> upstream/main          # 必须从上游最新切，不是从 request-audit
git worktree add /tmp/<功能名>-pr feat/<功能名>  # 独立工作树，不干扰主分支
cd /tmp/<功能名>-pr
git cherry-pick -x <功能提交...>                # 逐个挑，只挑该功能的提交
```

冲突逐个解完后，**在 PR 分支上重新执行**（不要沿用主分支的结果）：

```bash
cd backend && go generate ./cmd/server   # wire_gen.go 必须在干净基线重新生成
go build ./... && go vet ./... && go test -tags unit ./...
cd ../frontend && npx vue-tsc --noEmit && npx vitest run
```

### 三个必查项

每次都要查，因为它们不会以冲突的形式暴露出来：

1. **迁移编号是否与上游撞车**。上游在我们不知情时也在加迁移。`ls backend/migrations/*.sql | sort -t_ -k1 -n | tail -5` 对一眼，重号会让迁移 runner 的 checksum 校验直接失败。
2. **`cmd/server/wire_gen.go` 是否混入 fork 私有依赖**。这是生成文件，cherry-pick 会把请求审计、视频存储等注入一起带过去。必须 `git checkout HEAD -- ` 还原后重新 `go generate`。
3. **冲突块里是否裹着别的二开**。git 无法区分「本功能的代码」与「紧邻的其它二开代码」，自动合并常把后者一起并进来。解完所有冲突后全库扫一遍：

```bash
grep -rn "^<<<<<<< \|^>>>>>>> \|^=======$" --include="*.go" --include="*.ts" --include="*.vue" backend/ frontend/src/
for f in $(git diff --name-only upstream/main..HEAD); do
  grep -l "request_audit\|request_intercept\|video_storage\|VideoStorage" "$f" 2>/dev/null
done
git diff --name-only upstream/main..HEAD | grep -i "FORK_FEATURES"   # fork 私有文档不进 PR
```

### 时效性

上游推进很快（2026-08-15 当天就多了 8 个提交）。PR 分支切出后若隔了一段时间才提，**提交前重新 rebase 到 upstream/main 并完整重跑一遍验证**，否则等于又回到了「基线没校准」的状态。

## 2026-08-15：签到功能的上游 PR 分支与基线校准

签到功能已在本分支自用（提交 `2b0e35654`、`fbe88960e`）。为将来提 PR，另按上节流程切了一个只含签到的干净分支，**当前仅推送到 `origin`，尚未向上游提 PR**——先自用观察一段时间。

| 项 | 值 |
|---|---|
| PR 分支 | `feat/daily-checkin`（已推送 `origin`） |
| 切出基线 | `c204d33b0`（上游 main，`v0.1.176-9`） |
| 内容 | 2 个提交、38 个文件、+1567/-16，只含签到 |
| 验证 | 后端 build/vet/9 包 unit 测试全绿；前端 typecheck 通过、1547 测试全绿 |

前端测试数比本分支少 11 个，是 fork 私有功能的测试不在该分支内，属预期。

本次校准实际拦下三个问题，都不会以冲突形式暴露：

1. **迁移重号**：原用 `222`，但上游已有 `222_group_usage_daily_rollups.sql` 与 `223_group_usage_rollup_timezone.sql`，PR 分支改为 `224_user_checkin_records.sql`。**本分支仍是 `222`**，两边独立，合并上游时需注意本地 222 与上游 222 并存（迁移 runner 按文件名去重，不同名不冲突，但编号语义已错位，建议下次合并上游时把本地这条改名）。
2. **`wire_gen.go` 混入 5 处 fork 私有依赖**（请求审计、视频存储），已在干净基线重新生成。
3. **冲突块裹着其它二开**：`SettingsView.vue` 丢弃 216 行（请求审计 + 请求拦截两张卡片）、`domain_constants.go` 丢弃 12 个 fork setting key、`setting_parse.go` 丢弃 61 行 fork 私有辅助函数。

## 2026-08-13：清理根目录一次性中文文档

根目录三份一次性产出的中文文档全部删除，仓库文档收敛为「上游文档 + `docs/` 下两份二开文档」。三份都只在 `849ee52b6` 存在过，未被任何代码或配置引用，删除不影响构建与部署。

| 文档 | 删除提交 | 删除理由 |
|---|---|---|
| `CLOUDFLARE_PROTECTION_PLAN_CN.md` | `4c30d2b93` | 被同批的源码审计取代，且自身有两处会导致真实故障的错误：支付回调 SKIP 只覆盖 `/api/v1/payment/public/*`（浏览器 XHR），漏掉真正的服务器回调 `/api/v1/payment/webhook/*`；通配 `/api/*` 的 bypass-cache 会打死刻意标 `immutable` 的版本化公开设置资源。 |
| `SUB2API_CF_PROTECTION_AUDIT_CN.md` | 本次 | 一次性审计产出，Cloudflare 规则未执行。删除前已在 `1af6d8532` 完成全量复核（128 条引用中 99 条命中、27 条行号漂移、2 条失效），需要时取回的是已修订版本。 |
| `CLAUDE_CODE_OAUTH_GATEWAY_SPEC_CN.md` | 本次 | 未落地的外部项目任务书，本仓库无对应实现，其推荐目录结构一个都不存在；基线落后约 1300 个提交，行号锚点约半数失效。 |

取回方式：

```bash
git log --oneline --diff-filter=D -- '*_CN.md'          # 找到删除提交
git show <删除提交>^:SUB2API_CF_PROTECTION_AUDIT_CN.md   # 查看内容
git checkout <删除提交>^ -- SUB2API_CF_PROTECTION_AUDIT_CN.md  # 恢复文件
```

Cloudflare 防护的两份文档合计包含：完整路由矩阵、九条经复核仍成立的防护结论、十余条 CF 规则草案、源站 ufw 锁死脚本与 Caddy 真实 IP 配置。若后续真要落地 CF 防护，应从 `1af6d8532` 取回审计文档而不是重写。

## 2026-08-13：同步 main 至上游并重建二开清单

- `main` 分支直接同步为上游 `fbfdcef81`（原 `main` 停留在 v0.1.139 基线，含若干已被 `request-audit` 取代的本地 CI 提交）。同步前的旧 `main` 保留在标签 `backup/main-before-upstream-sync-20260813`。`origin/main` 未推送，如需远端也镜像上游需自行 force push。
- 以 `git diff upstream/main...request-audit` 全量核对二开差异（123 个文件，+18649/-137），逐项写入 [FORK_FEATURES.md](./FORK_FEATURES.md)。
- 核对发现并删除两节过期记录：订阅额度滚动窗口的生产代码已与上游一致（仅剩测试侧残留），备份失败闭环二开已由上游实现取代。
- `request-audit` 当前落后上游 4 个提交（`fd82dfd52`、`e29b93a1f`、`e215c98c2`、`fbfdcef81`，Grok 长上下文与媒体兜底修复），尚未合并。

## 2026-08-13：Gemini 图片模型接入 Images 与对象存储链路

本次功能提交为 `eb3a6bf40`。

### 背景与功能

Gemini 平台分组此前调用 `/v1/images/generations` 或 `/v1/images/edits` 会返回不支持，必须直接使用 `/v1beta` `generateContent`，图片通常以内联 Base64 返回，也无法复用现有图片对象存储转存能力。

本次增加 OpenAI Images 与 Gemini `generateContent` 之间的请求/响应适配，使 Gemini 图片模型能够使用已有同步及异步图片入口：

- 支持 Gemini 平台分组调用 `/v1/images/generations` 和 `/v1/images/edits`。
- 将 prompt、输入图片、尺寸档位和宽高比转换为 Gemini 图片请求；将 Gemini 内联图片转换回 OpenAI Images 响应结构。
- Gemini 异步图片任务复用已有任务状态、访问控制、安全审计、计费和对象存储上传链路。
- 对象存储启用时，生成结果可转存到 S3 兼容存储并返回预签名地址；未启用对象存储时，异步入口仍按原安全策略保持关闭。
- 空模型在异步提交阶段直接拒绝，避免创建必然失败的后台任务。

### 兼容性与数据影响

- 不改变 OpenAI、Grok 图片请求行为。
- 不改变 Gemini 原有 `/v1beta/*` 与 `/v1/chat/completions` 行为。
- 不修改支付、余额、模型计价配置或既有请求数据。
- 不新增 SQL、Ent Schema 或数据库迁移，不要求重建 PostgreSQL、Redis 或数据卷。
- 本次涉及 8 个后端文件，新增约 786 行、删除 1 行，其中包含适配器、handler、路由及定向测试。

### 验证与发布

提交 `eb3a6bf40` 的 GitHub CI 已通过 golangci-lint、前端检查、后端单元测试、集成测试及部署脚本检查；Security Scan 和 Docker 镜像构建也已通过。

生产升级仍必须仅更新 `sub2api`，不得联动重建数据库或 Redis。升级会重建应用容器并中断当前 SSE/WebSocket，需在低峰期经明确确认后执行。

## 2026-08-13：公开设置缓存与首屏载荷优化

本次功能提交为 `7520fa95a`、`b86f5243c`，兼容性修复提交为 `6e8e76717`。

### 背景与功能

旧 `/api/v1/settings/public` 包含 Base64 Logo 和完整法律协议正文，当前生产数据约 145.6 KiB。登录、注册等首屏只需少量展示和功能开关字段，却会下载全部内容，并可能重复触发公开设置的数据库读取。

本次调整包括：

- 为公开设置读取增加 5 秒进程内缓存、并发请求合并、3 秒数据库读取超时及设置更新主动失效。
- 新增 `/api/v1/settings/public/compact`，保留首屏需要的配置，将可转换的内联 Logo 和法律正文替换为版本化资源引用。
- 新增 `/api/v1/settings/public/logo/:revision` 和 `/api/v1/settings/public/legal/:revision/:document_id`，供前端按需加载并支持长期不可变缓存。
- 登录、注册和法律协议页面改用精简设置及按需资源。
- 不支持安全转换的内联 Logo 格式继续保留原值，避免配置特殊格式后出现 404。

### 兼容性与数据影响

- 旧 `/api/v1/settings/public` 保持完整响应，旧客户端无需同步升级。
- 不修改设置值、支付回调、模型网关、计费或用户数据。
- 不新增 SQL、Ent Schema 或数据库迁移，不要求重建 PostgreSQL、Redis 或数据卷。
- 首屏载荷按当前生产数据估算由约 145.6 KiB 降至约 5.5 KiB，约减少 96.2%。
- 载荷优化的两个提交合计涉及 8 个唯一文件，新增 278 行、删除 9 行；缓存提交独立保留定向单元与 race 测试。

### 验证与发布

提交 `7520fa95a`、`b86f5243c` 和 `6e8e76717` 均已通过对应 GitHub CI、集成测试、安全扫描和 Docker 构建。旧接口兼容、精简载荷大小、Logo 内容、法律正文及 HTML 注入场景另由受保护发布检查脚本覆盖。

生产部署后才能为上述新版本化路径启用 Cloudflare 缓存规则；部署前缓存工具的 preflight 必须因新端点尚未返回 200 而停止，不得提前修改线上缓存策略。

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

> 2026-05-10「订阅套餐额度窗口改为滚动周期」与 2026-05-19「合并上游 v0.1.127 并完善备份失败闭环」两节已于 2026-08-13 删除：对应二开分别被上游自行实现的按自然日对齐窗口和上游备份/对象存储实现取代，本地已无等价代码，保留记录会与现状冲突。
