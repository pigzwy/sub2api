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

## 2026-09-01：合并上游 v0.1.185

本次将 `upstream/main` 从 `52374af94` 推进到
`a2fb09260a955676f99cdc92f05469febee82a08`（`v0.1.185`），共 26 个提交、
46 个上游变更文件。上游主要增加价格目录 `override_file` 覆盖与数据驱动长上下文
阶梯、Codex priority tier、WebSocket 容量降载/陈旧连接回收、数据库启动重试，
并修复账号统计计价、API Key instructions 和 delegation bootstrap；这些均采用
上游实现。

唯一文本冲突是 `backend/internal/service/billing_service.go`。冲突两侧不是重复
功能：上游在同一结构体初始化处加入 xAI 长上下文“达到阈值即进高档”的语义，
二开在这里接入 OpenAI Realtime 的音频输入、音频输出和音频缓存读取三档价格。
处理时以上游新的目录驱动计价管线为主体，完整保留
`LongContextThresholdInclusive`，同时保留二开的
`AudioInputPricePerToken`、`AudioOutputPricePerToken` 和
`AudioCacheReadPricePerToken` 字段；旧的硬编码长上下文规则没有恢复。

合并后相对上游仍为 233 个二开差异文件（+25,421 / -396 行）。请求审计/拦截、
视频和音频 S3、Gemini 图片、OpenAI Realtime、签到、Studio 模型价格接口和静态
SPA 壳的关键装配均在。本机不执行 Go/前端重型构建，推送后由 GitHub Actions 的
unit、integration、frontend、golangci-lint、security scan 与镜像构建验证。

## 2026-08-31：合并上游 v0.1.184

本次将 `upstream/main` 从 `00d011186` 之后的 170 个提交合入
`request-audit`，上游基线为 `52374af94031f04df8de6fc91deb77a179e04b06`
（`v0.1.184`）。合并结果相对上游保留 233 个文件的独有二开差异（新增
约 25,396 行、删除 394 行）；上游新增的 Codex 路由目录、推理强度与原生
compaction、WebSocket/Realtime、配额与计费修复、模型目录和前端修复均采用
上游实现。

本次有 5 个文本冲突，全部集中在用量日志仓储及 OpenAI 用量记录：

- `backend/internal/repository/usage_log_repo_insert.go`
- `backend/internal/repository/usage_log_repo_query.go`
- `backend/internal/repository/usage_log_repo_request_type_test.go`
- `backend/internal/repository/usage_log_session_id_unit_test.go`
- `backend/internal/service/openai_gateway_usage.go`

处理方式是以上游列序、查询和计费逻辑为主体，同时保留本分支独有的
Realtime 音频用量字段（`audio_input_tokens`、`audio_input_cost`、
`audio_output_tokens`、`audio_output_cost`），并同步更新单条/批量写入、扫描和
测试 fixture。上游新增的 `requested_reasoning_effort` 与
`native_compaction_v2` 字段也完整保留。未发生迁移文件名冲突；上游本身存在多个
同数字前缀的迁移，runner 按完整文件名排序并按文件名记录校验和，本次未改动其
命名或内容。本地未执行 Go 构建、测试或前端构建，质量验证交由推送后的 GitHub
Actions CI。

本次合并后继续保留且复核装配的独有功能包括请求审计/拦截、视频和音频 S3
转存、Gemini 图片入口、OpenAI Realtime、签到、模型价格接口和静态 SPA 壳。
若上游后续提供其中任一等价实现，按本文件开头的规则整套切换到上游版本。

## 2026-08-25：合并上游 v0.1.183

`upstream/main` 到 `7634e3c23`（`v0.1.183`），9 个实质提交，**零冲突且与二开零重叠**
（上游改的 24 个文件与本分支改动集无交集）。内容为 OpenAI OAuth 配额耗尽 429 自动暂停、
Codex session-id 头、容量溢出时保持粘性绑定、工具调用 item ID 保持类型、Kimi 并发 403 可恢复、
Antigravity token 上限收敛、channel-monitor-v2 复合平台聚合 SQL、邮箱换绑并发守卫。

验证按 `backend-ci.yml` 逐条对齐：build、vet、`-tags=unit`（= `make test-unit`）、无 tag 全量、
`golangci-lint run ./...`（0 issues）、`-tags=integration` 与 `-tags=e2e` 编译、
前端 247 文件/1780 用例与 `vue-tsc --noEmit`，全部通过。二开功能十三项逐条核验均在。

**已知的既有 gofmt 噪声**：`internal/handler/auth_current_user_test.go` 未格式化，但**上游自己的
版本同样如此**，二开从未改过该文件——核对 gofmt 结果时可忽略，不要误当成本地引入。

## 2026-08-25：合并上游 v0.1.182

`upstream/main` 到 `aa2c4e8d1`（`v0.1.182`），14 个实质提交，**零冲突**。主要是 OpenAI
Responses Lite 的工具调用/数值精度修复、Antigravity Sonnet 路由、Anthropic 缓存 TTL 重复
计费修复、Kimi Code K3 复合路由、支付履约后刷新余额。

上游本次改的 48 个文件里有 4 个与二开重叠（`openai_images.go`、`account_test_service.go`、
`billing_service.go`、`composite_platform_test.go`），git 全部自动合并。其中前两个正是
gpt-image `response_format` 修复所在处——已逐条确认三处修复（转发 JSON 分支、转发 multipart
分支、后台测试路径）均完好；上游对 `openai_images.go` 的改动只是新增 OAuth 逐字提示词常量，
与该修复不相干。

验证按 `backend-ci.yml` 的步骤逐条对齐：`go build ./...`、`go vet`、
`go test -tags=unit ./...`（= `make test-unit`）、`go test ./...`、
`golangci-lint run ./...`（0 issues）、`-tags=integration` 与 `-tags=e2e` 编译、
前端 `vue-tsc --noEmit` 与 247 文件/1780 用例，全部通过。

**环境注意**：本机 `/tmp` 是 2.9 GB 的 tmpfs，Go 链接 `cmd/server` 时会因空间不足报
`link: mapping output file failed: no space left on device`。这不是代码问题，导出
`TMPDIR=/root/gotmp`（落在磁盘上）即可正常编译。

## 2026-08-24：合并上游 v0.1.181

`upstream/main` 到 `e2d9b823f`（`v0.1.181`）。含 4 个实质提交：Grok 改用官方 CLI user agent、
OpenAI Responses 按 item 类型清理 rejected 状态、Gemini 清洗不受支持的 tool schema 字段。
**零冲突**——上游本次改的 14 个文件里只有 `service/openai_gateway_grok_test.go` 与二开重叠，
git 自动合并成功，二开加在该文件里的两个测试
（`TestForwardGrokMedia{ImagesEdit,Video}MultipartAppliesCompositeAndAccountModelMapping`）均存活。

验证：`go build ./...` 通过、后端 `go test ./...` 全绿、gofmt 无新增问题；
[FORK_FEATURES.md](./FORK_FEATURES.md) 的二开功能逐项核验均在。

**过程记录（值得下次注意）**：合并前查上游状态时，`git fetch` / `git ls-remote` 一度持续返回
陈旧的 `03e8ab413`（GitHub git 前端副本延迟），据此误判为"无更新"；改用 GitHub API 查
`/commits/main` 才发现 main 已到更新的提交。**判断上游是否有更新时不要只信 git 一个源**，
与 API 交叉核对。另外该版本的 Release 被删除后重新发布过（`created_at` 07:16 / `published_at` 11:58），
git tag 对象未变，仅从 tag 看不出来。

## 2026-08-24：合并上游 v0.1.180

`upstream/main` 推进到 `03e8ab413`（`v0.1.180`）后整体合并，合并后本分支不再落后上游。
共 480 个文件变更，9 个文件冲突，按本文件开头的规则梯度（上游正式实现 > 本地旧二开）逐个处理。

**淘汰二开、改用上游实现**

- `service/openai_gateway_scheduling.go`：上游自己做了一版 eligibility reason 重构
  （`openAICompatibleAccountEligibilityFailureReason`，返回 `""` 表示可用），与二开此前的
  `(bool, string)` 版本重复。按规则 2 整文件取上游，并把二开独有调用点
  `service/openai_realtime.go` 的 `summarizeOpenAIRealtimeSchedPool` 适配到新 API。
  **能力损失（已知且接受）**：二开原本能把硬配额耗尽细分到
  `account_quota_exceeded_{total,daily,weekly}`，上游统一报 `not_schedulable`；
  `quota_auto_pause_<window>` 是另一套阈值自动暂停机制，不能替代。两个相关测试改为
  锁定仍然成立的准入保证（三种配额窗口耗尽都必须被拒），不再断言窗口级归因。
  核验：`git diff upstream/main -- backend/internal/service/openai_gateway_scheduling.go` 为空。
- `handler/grok_audio.go`：上游重写了 Grok Realtime（4 次重试的选号 + 握手前探测
  `OpenGrokRealtime`，代理改用 `ProxyGrokRealtimeConn`），比二开版更健壮。取上游实现，
  仅保留二开独有的利润门抑制（Realtime 按音频分钟计费，文本利润门不适用），
  并把计费调用对齐到上游的 `model` 变量。二开的 `grokRealtimeSelectionModel`
  被上游"空模型 + 能力选路"取代。

**两边独立新增、仅文本相撞，全部保留**

- `config/config.go`：二开 `VideoStorage` + 上游 `Plugins`。
- `pkg/xai/models.go`：取上游的 video-1.5 条目（注意上游把
  `DefaultImagineVideo15Model` 与 `DefaultImagineVideo15LegacyModel` 的含义对调了），
  保留二开的 `grok-voice-latest` 目录条目（上游有语音功能但未登记到模型目录）。
- `service/openai_gateway_grok_test.go`：上游新增 7 个测试 + 二开 2 个，合并保留。
- `frontend/package.json`：二开 `nanoid` 安全 pin + 上游 `dompurify` pin。
- `frontend/src/api/admin/index.ts`：二开 `requestAudit` + 上游 `plugins`。

**生成物重新生成而非手工合并**

- `cmd/server/wire_gen.go`：用 `go run github.com/google/wire/cmd/wire ./cmd/server/` 重新生成，
  同时装配二开（requestAudit / checkin / modelPricingResolver / videoStorage）与
  上游（plugins / openAIQuotaAutoReset）的 provider。
- `frontend/pnpm-lock.yaml`：取上游版后按合并的 `package.json` 执行
  `pnpm install --lockfile-only` 重新生成，两个 override 均在。

**验证**：`go build ./...`、`go vet`、后端 `go test ./...` 全绿；前端
`vue-tsc --noEmit`、`eslint`、247 个测试文件 / 1779 个用例全绿。
[FORK_FEATURES.md](./FORK_FEATURES.md) 的十项二开功能逐项核验均在。

## 2026-08-19：同步生产源码基线并加固静态 SPA 壳

生产工作树先通过只读 `fetch` 与远端 `origin/request-audit` 快进核对到
`3cb9558ff`，没有强制覆盖远端，也没有部署或编译。该基线包含 OpenAI
Realtime 的能力提示和机器可读 503 归因修复，以及上游截至当时的迁移变更。

在保留已有未提交二开差异的前提下，新增一组小范围的首屏/路由防护改动：

- 后端改用不注入请求级配置的稳定嵌入式 HTML 壳；公开设置由前端通过现有
  `/api/v1/settings/public` 请求加载，首屏最多等待 2.5 秒，故障时仍可挂载。
- HTML 壳增加稳定内容 ETag 和 60 秒 `must-revalidate` 缓存；版本化静态
  资源继续使用长期 immutable 缓存。
- SPA fallback 限定为 `GET`/`HEAD`。页面路径的写请求不再被返回 `200
  index.html`，而是继续进入正常 Gin 路由，避免伪造成功响应并减少无效源站工作。
- 增加后端定向测试覆盖壳缓存、ETag、运行时配置不注入和写方法边界。
- 2026-08-20 复核生产响应时发现，HTML 虽已不含内联 nonce，通用 CSP
  中间件仍会在响应头生成请求级 nonce。静态壳发送前现会移除这一个无用途的
  nonce source，同时保留其余 CSP 来源与安全指令；新增测试锁定该行为。该补丁
  仅进入源码/CI 流程，镜像更新并验证前不得扩大 Cloudflare SPA 缓存。

涉及文件：

```text
backend/internal/server/router.go
backend/internal/web/embed_off.go
backend/internal/web/embed_on.go
backend/internal/web/embed_test.go
backend/internal/web/static_cache.go
backend/internal/web/static_cache_test.go
frontend/src/main.ts
```

验证边界：本机仅执行 `git diff --check`；当前生产机没有 Go 工具链和前端
依赖，且按约定不得在生产机编译或构建 Docker 镜像。提交推送后由 GitHub
Actions 完成后端/前端检查和镜像构建；部署仍需低峰期单独确认，不得重建数据库、
删除数据卷、重启现有生产服务或执行 Cloudflare 写操作。

### 当前仍需维护的独有二开（2026-08-19 更新）

完整清单见 [FORK_FEATURES.md](./FORK_FEATURES.md)，含每项功能的入口、接口、配置、文件清单、测试和合并冲突高发点。概览：

- 请求审计：按用户、分组和保留期记录并查询网关请求/响应。
- 请求内容拦截：命中配置规则后返回本地模拟响应，不请求上游模型。
- 管理统计自定义日期区间：订单支付概览和账号使用统计支持开始、结束日期。
- Grok 视频对象存储：完成后流式转存到独立 S3 目标，并返回预签名链接。
- 公开设置性能优化：进程内短缓存、精简首屏载荷、版本化 Logo 和法律正文按需加载。
- Gemini 图片兼容：Gemini 图片模型可复用 OpenAI Images/异步任务入口，并接入现有对象存储转存链路。
- 自定义菜单打开方式：每个菜单项可选 iframe 嵌入、当前标签页跳转或新标签页打开。
- 每日签到（活动）：每天一次随机余额奖励，金额区间与人机验证可配；已另切 `feat/daily-checkin` 作为上游 PR 分支。
- OpenAI Realtime 语音网关：`/v1/realtime` 平台分流与 WS 直通、逐回合 token 计费、audio 计费维度、分组开关与账号能力、语音型号入默认模型表、账号 Realtime 连通性测试。
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

## 2026-08-19：OpenAI Realtime 语音网关与语音运营工具

两轮特性分支，均按「本机不构建」约定在 GitHub Actions 完成 ent codegen / 构建 / 测试后合入 `request-audit`：

- `feat/openai-realtime-voice` → 合并 `84519d1f6`：`GET /v1/realtime` 按分组平台分流（grok 沿用上游按分钟计费不动，openai 新增 WS 直通）、`groups.allow_realtime` 开关（迁移 `227`，含鉴权缓存投影与 `GetByKeyForAuth` 列投影）、账号 `realtime` 能力（apikey-only 显式勾选）、audio token 计费维度全链路（迁移 `228`，usage_logs 追加式列序 + 守护测试适配 63 列）、逐回合 `response.done` 落账（`request_id=openai_realtime:<response.id>`）、价格表补 `gpt-realtime-2.1`、新增 `.github/workflows/entgen.yml`（CI 生成 ent 代码并以 `[skip ci]` 回推）。
- `feat/realtime-admin-tooling` → 合并 `64db9a319`：语音型号进默认模型表（openai：`gpt-realtime` / `-2.1` / `-mini`；xai：`grok-voice-latest`。只加分组候选不够——`/v1/models` 自定义清单的兜底 source 来自 `DefaultModels`，不在表内会被过滤）、OpenAI 账号「实时语音 Realtime」测试模式（对照既有 Grok realtime 探测：`session.created` 判可用、`error` 事件判失败、零 token 消耗）。

功能详情、文件清单与冲突高发点见 [FORK_FEATURES.md](./FORK_FEATURES.md) 第 11 节。对接方 Pig Studio 工作台语音页同步上线（其仓库 main：`7f4ad7e` 语音功能、`fc6324e` 裸端口 WS 直连逃生口），网关接口契约双侧钉死并有测试锁定。

**过程中发现的既有问题（与本功能无关，待单独处理）**：`TestGroupUsageRollupTrigger*` 两个集成测试在东八区凌晨 0–8 点窗口（UTC 16–24）必然失败——rollup 触发器在「上海已过午夜而 UTC 未过」时不推进水位；此前 CI 均在白天时段运行故从未暴露，同一提交出窗口重跑即绿。

## 2026-08-15：合并上游 v0.1.177

本次上游基线为 `baeac1f3d`（`v0.1.177`），共 17 个提交、76 个文件。**自动合并无冲突。**

上游主要内容：分组用量日汇总（新增迁移 `222`、`223`）、Codex turn-state 中继与指纹收敛改为 opt-in、原生/旧版 compaction 路由分离、Grok 长上下文与媒体兜底修复、Go 版本升至 1.26.6。

与二开重叠的文件只有 7 个：`backend/go.mod`、`config/config.go`、`handler/openai_gateway_handler.go`、`service/openai_gateway_service.go`、`frontend/pnpm-lock.yaml`、`i18n/locales/{zh,en}/admin/overview.ts`。其中 `openai_gateway_handler.go` 是二开侵入最深的文件（请求审计埋点与请求拦截判断都在其中），合并后逐条核对了调用顺序：审计捕获 → 安全审计 → 拦截判断，与合并前一致。

**迁移编号撞车已处理**：上游新增的 `222_group_usage_daily_rollups.sql` 与本地 `222_user_checkin_records.sql` 同号。迁移 runner 按文件名排序执行、按文件名去重，两者并存不会冲突，但编号语义已乱，故把本地这条改名为 `224_user_checkin_records.sql`（与 PR 分支 `feat/daily-checkin` 一致）。

> **升级注意**：已部署过旧版本的实例，`schema_migrations` 里记录的是 `222_user_checkin_records.sql`；改名后 runner 会把 `224_...` 当作新迁移**再执行一次**。该迁移全部使用 `CREATE TABLE IF NOT EXISTS` / `CREATE UNIQUE INDEX IF NOT EXISTS`，重跑无副作用，只会在 `schema_migrations` 多留一行记录。签到数据不受影响。

**上游遗漏，已在本分支修正**：v0.1.177 把 `backend/go.mod` 升到 1.26.6，也改了四处 workflow 的 `go version | grep -q` 断言，**但三个 Dockerfile 仍是 `golang:1.26.5-alpine`**。官方 golang 镜像带 `GOTOOLCHAIN=local`，不会自动下载更高版本工具链，于是镜像构建在 `go mod download` 直接失败：

```
go: go.mod requires go >= 1.26.6 (running go 1.26.5; GOTOOLCHAIN=local)
```

本分支已把根 `Dockerfile`、`deploy/Dockerfile`、`backend/Dockerfile` 一并升到 `golang:1.26.6-alpine`。这是上游自身的缺陷（上游镜像构建同样会失败），与二开无关；若上游后续自行修复，合并时以上游为准即可。

二开功能核验（合并后逐项确认埋点仍在）：请求审计 20 处、请求拦截 138 处、视频转存 21 处、Gemini 图片 6 处、签到 17 处；二开独有文件全部存在；中英文签到文案键集完全一致（各 13 个）。

验证：后端 `go build ./...`、`go vet ./...`、`go test -tags unit ./...` 全量通过；前端 typecheck 通过、构建通过、224 个测试文件 1560 个测试全绿（较合并前多 2 个，为上游新增）。

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
