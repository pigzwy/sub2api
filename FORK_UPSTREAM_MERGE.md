# Fork 上游合并操作手册

> 本文档是 `pigzwy/sub2api` 在合并上游 `Wei-Shaw/sub2api` 时的长期策略和操作 SOP。
>
> **用途**：每次准备合并上游前，把本文档丢给 AI，让 AI 先按此执行"合并前分析"，再按"合并执行方案"落地。
>
> 与 `FORK_DEV_GUIDE.md` 的区别：
> - `FORK_DEV_GUIDE.md`：**怎么写**二开代码才不会和上游冲突
> - 本文档：**怎么吃**上游代码才能保留要的、扔掉不要的

---

## 0. AI 快速指引（每次合并前 AI 必读）

你（AI）被调用来帮助维护 `pigzwy/sub2api` fork，准备合并上游 `Wei-Shaw/sub2api` 的最新版本。

**执行步骤：**

1. 先读完本文档所有章节（尤其第 2、3、4、5 节的白/黑名单）
2. 按第 6 节的"合并前分析脚本"执行分析，输出一份报告给用户：
   - 本次合并范围（commit 数量、merge base、目标版本）
   - 命中黑名单的 commit 清单（哪些是支付相关要跳过）
   - 命中灰色地带的 commit 清单（需用户决策）
   - 白名单默认接收的 commit 清单
   - 推荐的合并方案（A 或 B）
3. **等用户确认后**再按第 7 节执行合并命令
4. 合并完成后跑第 8 节的验证清单
5. 验证出问题时回退到第 9 节的回滚预案

**全局规则提醒：**
- 这个仓库的用户 git 规则里 **禁止** AI 直接执行 `git commit / merge / push / reset / rebase`。所有写操作都要输出命令让用户自己跑，或使用 Bash 允许范围内的只读命令分析。
- 工具使用上强制：`Read/Glob/Grep/Edit/Write` 优先，不要用 `cat/find/grep/sed` 这些系统命令替代。
- 语言使用中文。

---

## 1. Fork 身份 & 长期策略

| 项目 | 值 |
|---|---|
| 上游仓库 | `https://github.com/Wei-Shaw/sub2api` (remote: `upstream`) |
| 本 Fork | `https://github.com/pigzwy/sub2api` (remote: `origin`) |
| 默认主分支 | `main` |
| 合并节奏 | 跟随上游 release（通常关注 VERSION tag，如 `v0.1.111`） |

**长期策略核心：**

1. **跟进上游的 bug 修复、安全补丁、基础架构改进**
2. **拒绝上游内置支付系统接管主入口 / 主充值链路，但允许相关代码在不影响外挂支付时合入静置**（本 Fork 使用自写的外部支付页 + iframe 嵌入，见第 3 节详解）
3. **对大型新 feature 保持审慎**，按需决策
4. **尽量少改动上游代码**（已有改动见第 2 节），便于长期合并

---

## 2. 必须保留的 Fork 专属改动（合并时不能被覆盖）

这些是 fork 上独有、upstream 永远没有的改动。每次合并后要验证它们仍在。

### 2.1 Fork 专属文件（upstream 根本不存在）

| 文件 | 用途 |
|---|---|
| `.github/workflows/fork-docker-build.yml` | Fork 自己的 Docker 构建 workflow |
| `AI-CLI-Guide.md` | Fork 维护的 AI CLI 使用文档 |
| `FORK_DEV_GUIDE.md` | Fork 开发指南（如何写二开代码） |
| `FORK_UPSTREAM_MERGE.md` | **本文档** |

> 这些文件只存在于 fork HEAD，不存在于 merge base 和 upstream。三方合并会保留它们，**不用特殊处理**。验证方法：合并完 `git ls-files` 能看到它们即可。

### 2.2 对 upstream 文件的精准补丁

| 文件 | 行为 | 上游原值 | Fork 修改值 |
|---|---|---|---|
| `backend/internal/service/update_service.go` | `githubRepo` 常量 | `"Wei-Shaw/sub2api"` | `"pigzwy/sub2api"` |

**验证命令：**

```bash
grep -n 'githubRepo' backend/internal/service/update_service.go
# 必须输出：githubRepo = "pigzwy/sub2api"
```

如果合并后这里变回了 `Wei-Shaw/sub2api`，说明 upstream 又碰过这行，需要手动复原 fork 的改动。

### 2.3 如果将来新增了 fork 补丁

在本节表格追加一行，格式：`文件 / 行为 / 上游值 / Fork 值`。每次合并 AI 都按此表核对。

---

## 3. 黑名单 — 拒绝的 upstream 功能

### 3.1 为什么拒绝：内置支付系统 v2

上游在 v0.1.111 引入了完整的内置支付系统（PR #1572 `feat/payment-system-v2`），支持 Stripe / Alipay / WxPay / EasyPay。

**本 Fork 不接受它接管当前充值主链路**，原因：
- 本 Fork 的支付是**外部自写支付页**，通过 iframe 嵌入到用户界面
- 嵌入通道：`frontend/src/utils/embedded-url.ts` 里的 `buildEmbeddedUrl()`，给外部页面传 `user_id / token / theme / lang / ui_mode=embedded / src_host / src_url` 查询参数
- 嵌入位置：旧版用 `PurchaseSubscriptionView.vue`；如果 upstream 已经把它删了，用 `CustomPageView.vue` (`/custom/:id`) 也能嵌入同一套参数

### 3.1.1 2026-04 现行判断：为什么继续拒绝 payment v2

经过一次实际排查后，当前结论更新为：

- **不是接口层硬冲突**：本 Fork 的外部支付成功后，实际上是通过 `admin_api_key` 调后端的管理员接口 `/admin/redeem-codes/create-and-redeem` 完成充值；这条链路本身可以独立运行，不依赖 upstream payment v2
- **真正的冲突在入口层和账务模型层**：
  - upstream payment v2 会接管 `/purchase`，把当前 iframe 入口替换成 `PaymentView.vue`
  - upstream payment v2 引入 `payment_order / payment_provider / subscription_plan` 等完整订单模型
  - 本 Fork 现有外部支付仍然以“固定兑换码 + 立即兑换”作为到账模型，两套系统并存会形成双账本、双入口、双退款语义
- **因此长期策略更新为**：
  - 继续拒绝 payment v2 接管 `/purchase`、`purchase_subscription_*`、`buildEmbeddedUrl()` iframe 入口以及当前外挂支付主充值链路
  - 继续保留 fork 的外部支付为唯一主充值入口
  - 若 upstream 的 payment 相关代码已经合入，但**没有替换上述入口、没有破坏外挂支付链路、没有导致程序启动失败**，默认可先保留，不做机械清理
  - upstream 在共享文件里的非支付优化仍然要接；必要时人工解冲突或手工移植，但处理目标是**保住外挂支付入口**，不是按文件名把整套 payment 代码全删掉

### 3.2 黑名单 — 命中即重点审查，不再按文件名自动整块删除

#### 3.2.1 文件 glob 模式（任一命中即认为是支付相关高风险变更）

> **注意**：下面这些模式是“高风险信号”，表示需要重点确认是否会接管外挂支付主链路；**不是**“命中就必须删除”的自动清理清单。

```
backend/ent/payment*                                     # ent 生成的支付相关表
backend/ent/paymentauditlog/**
backend/ent/paymentorder/**
backend/ent/paymentproviderinstance/**
backend/ent/subscriptionplan/**
backend/ent/schema/payment_*.go
backend/ent/schema/subscription_plan.go
backend/internal/payment/**                              # 支付业务包
backend/internal/handler/payment_*.go
backend/internal/handler/admin/payment_handler.go
backend/internal/service/payment_*.go
backend/internal/server/routes/payment.go
backend/migrations/*purchase*.sql                        # purchase_subscription 迁移脚本
backend/migrations/*payment*.sql

frontend/src/views/user/Payment*.vue                     # 内置支付前端页
frontend/src/views/user/Stripe*.vue
frontend/src/views/user/UserOrdersView.vue
frontend/src/views/admin/orders/**                       # 订单管理页
frontend/src/stores/payment.ts
frontend/src/types/payment.ts
frontend/src/assets/icons/alipay.svg
frontend/src/assets/icons/wxpay.svg
frontend/src/assets/icons/stripe.svg
frontend/src/assets/icons/easypay.svg

docs/PAYMENT*.md                                          # 内置支付文档
```

#### 3.2.2 commit message 关键字（任一命中即需审查）

```
feat(payment)
fix(payment)
refactor(payment)
feat(settings):.*(ZPay|EasyPay|Alipay|WxPay|Stripe|payment)
feat(stripe)
feat(alipay)
feat(wxpay)
feat(easypay)
Stripe
PaymentOrder
purchase_subscription
payment-system-v2
payment-docs
```

#### 3.2.3 已知必须拒绝的 upstream PR（长期更新）

| PR | Merge SHA | 说明 |
|---|---|---|
| #1572 `feat/payment-system-v2` | `97f14b7a` | 内置支付系统主体 |
| #1576 `feat/payment-docs` | `54490cf6` | 支付文档 + SettingsView 里的 ZPay/EasyPay 推荐链接 |
| #1610 `fix/alipay-wxpay-type-mapping` | `7d80b5ad` | Alipay/Wxpay 提供方映射 + 跨渠道负载均衡（f498eb8f） |
| #1612 `fix/qrcode-density` | `75908800` | 支付二维码密度降低（改 PaymentQRDialog/PaymentStatusPanel/PaymentQRCodeView） |

**以后遇到新的支付相关 PR 要追加到本表，AI 每次合并前读本表就知道哪些改动需要重点拦截“接管入口”的风险。**

**注意**：
- 黑名单 PR 的“拒绝”优先指向**入口接管、配置替换、主链路替换**，而不是要求把所有 payment 相关代码从仓库里删干净
- 如果某个 payment PR 已经合进来，但当前 fork 仍保住了 `/purchase`、`purchase_subscription_*` 和外挂支付链路，则**不要为了匹配旧规则再去机械 `git rm` / revert 整套 payment 模块**
- 只有当某个 PR 真的让外挂支付入口失效、程序启动失败，或引入了不能接受的账务语义时，才进入回滚 / 局部还原流程

### 3.3 黑名单的边界：已被 upstream 移除的"旧 iframe 入口"

upstream 在 v0.1.111 已经**删除**了 `frontend/src/views/user/PurchaseSubscriptionView.vue`，并把路由 `/purchase` 改成指向新的 `PaymentView.vue`。我们的处理方式是：

- 如果选**方案 A**（merge 后保留上游代码、只修入口）：优先手工恢复 `/purchase -> PurchaseSubscriptionView.vue`、`purchase_subscription_*` 和 iframe 参数链路，不主动清理其余 payment 代码
- 如果选**方案 B**（只回滚接管点）：仅回滚会替换外挂支付入口的路由 / 设置 / 页面，不追求把所有 payment 相关文件删干净
- 如果未来某天不得不接受删除：切换到用 `custom_menu_items` + `CustomPageView.vue`（`/custom/:id`）做 iframe 嵌入，query 参数构造逻辑是一样的

---

## 4. 白名单 — 默认接收的 upstream 改动

**凡是不命中黑名单的，默认全都要。** 以下类型的 commit 一律接收：

| 类型 | 示例 | 说明 |
|---|---|---|
| 安全/依赖升级 | `fix(deps): upgrade axios`, `fix: bump Go ...` | 必须接 |
| Bug 修复 | `fix: ...`, `fix(ui): ...`, `fix(account): ...` | 默认接 |
| Lint / Test 修正 | `fix(lint): ...`, `fix(test): ...` | 默认接 |
| 文档 | `docs: ...`（除非属于黑名单里的支付文档） | 默认接 |
| 重构（非支付） | `refactor: ...`（非 payment 包） | 默认接 |
| Chore | `chore: update sponsors`, `chore: sync VERSION` | 默认接 |

---

## 5. 灰色地带 — 需要用户决策

某些大 feature 虽然不是支付，但会**碰到和支付 PR 重叠的共享文件**（如 `SettingsView.vue`、`router/index.ts`、`handler/wire.go`、`i18n/*.ts`、`AppSidebar.vue`）。这类 commit 单独 cherry-pick 可能不行，但跟着 merge + revert 的流程可能能干净地带进来。

**默认策略：接受。** 除非和黑名单 PR 有严重的文件重叠导致无法干净处理，否则大 feature 一律接受，不需要每次都问用户。

### 5.1 已确认接受的大 feature（长期更新）

| Feature | 上游 commit / PR | 状态 |
|---|---|---|
| OIDC 登录 | `02a66a01` + `8e1a7bdf` + PR #1010 | ✅ 已确认接受 |
| 表格排序/搜索转后端 | `5f8e60a1` + `ad80606a` | ✅ 默认接受 |
| Messages 调度映射 | `23c4d592` + `4de4823a` + `de9b9c9d` | ✅ 默认接受 |

> 以后遇到新的大 feature，如果能干净 merge 进来就直接接，只有在 merge/revert 后构建失败时才升级为"需用户决策"。

### 5.2 决策流程

对每个灰色地带 commit，AI 要：

1. 用 `git show --stat <sha>` 列出它改了哪些文件
2. 判断是否和当前黑名单 PR 有文件重叠
3. 在分析报告里写明：**"这个 commit 动了 N 个文件，其中 M 个和支付 PR 重叠"**
4. **默认接受**，除非 merge 后会替换外挂支付主链路、导致程序起不来，或用户明确要求剔除

---

## 6. 合并前分析脚本（AI 执行）

每次合并前，AI 按顺序执行这些命令，输出分析报告。

### Step 1 — 拉取 upstream

```bash
git fetch upstream --tags
git log --oneline -5 upstream/main
cat backend/cmd/server/VERSION  # 当前本地版本
git show upstream/main:backend/cmd/server/VERSION  # upstream 版本
```

### Step 2 — 确定合并范围

```bash
# merge base
git merge-base HEAD upstream/main

# 本次要合并的 commit 数
git rev-list --count HEAD..upstream/main

# 本次要合并的 commit 列表（完整）
git log --oneline HEAD..upstream/main
```

### Step 3 — 检测文本级冲突

```bash
# git merge-tree 如果只输出一个 tree SHA 就是无冲突
git merge-tree --write-tree HEAD upstream/main
```

- 只有 tree SHA：文本层无冲突，可以走方案 A
- 输出带冲突段：记录冲突文件清单，可能要走方案 B

### Step 4 — 扫描黑名单命中

```bash
# 4.1 按文件 glob 扫描
git diff --name-only HEAD upstream/main | grep -iE \
  'payment|purchase_subscription|subscriptionplan|stripe|alipay|wxpay|easypay|/orders/'

# 4.2 按 commit message 扫描
git log --oneline HEAD..upstream/main | grep -iE \
  'payment|purchase_subscription|stripe|alipay|wxpay|easypay|zpay'

# 4.3 核对已知黑名单 PR 是否在范围内
for sha in 97f14b7a 54490cf6; do
  if git merge-base --is-ancestor $sha upstream/main \
     && ! git merge-base --is-ancestor $sha HEAD; then
    echo "命中黑名单 PR: $sha"
  fi
done
```

### Step 5 — 核对 fork 专属补丁是否被动过

```bash
# update_service.go 是否被 upstream 动过
git log HEAD..upstream/main --oneline -- backend/internal/service/update_service.go
# 如果有输出，需要手动核对是否碰到了 githubRepo 那一行
```

### Step 6 — 输出分析报告（模板）

AI 汇总前面所有结果后，按以下格式输出给用户：

```markdown
## 本次合并分析报告

- **目标版本**：v0.1.xxx（upstream/main）
- **本地版本**：v0.1.yyy
- **Merge base**：<sha>
- **Commit 数**：N 个

### ✅ 默认接收（白名单）：M 个
- <sha> <subject>
- ...

### ❌ 必须跳过（黑名单）：K 个
命中的已知 PR：
- #1572 feat/payment-system-v2 (merge: 97f14b7a)
- #1576 feat/payment-docs (merge: 54490cf6)

命中的其他支付 commit：
- <sha> <subject>
- ...

### ⚠️ 需要决策（灰色地带）：L 个
- <sha> <subject>
  - 改了 N 个文件，其中 M 个和支付 PR 重叠
  - 建议：接 / 跳 / 问

### 🔧 Fork 专属补丁
- update_service.go: 被 upstream 动过吗？是 / 否
- 其他 fork 专属文件：无/有

### 📋 推荐方案
- 方案 A（merge + revert）/ 方案 B（cherry-pick）
- 理由：...

### 🚦 请用户确认
"是否按方案 X 执行？需要我先跑哪步？"
```

---

## 7. 合并执行方案

### 7.1 方案 A：merge upstream，保留 payment 代码静置，仅守住外挂支付入口（推荐）

**适用条件：**
- Step 3 显示文本层无冲突
- 或冲突集中在入口类文件，可人工保住外挂支付链路

**执行步骤：**

```bash
# 1. 开安全分支，绝不动 main
git checkout -b merge/upstream-vX.Y.Z

# 2. 合并 upstream（verify 过无冲突才跑这步）
git merge --no-ff upstream/main \
  -m "Merge upstream vX.Y.Z (preserve external payment entry)"

# 3. 只处理会影响外挂支付的热点文件
#    重点检查：
#    - frontend/src/router/index.ts
#    - frontend/src/views/user/PurchaseSubscriptionView.vue
#    - frontend/src/utils/embedded-url.ts
#    - backend/internal/handler/dto/settings.go
#    - frontend/src/views/admin/SettingsView.vue
#    - backend/internal/service/update_service.go
#
#    目标：
#    - /purchase 仍指向外挂支付入口
#    - purchase_subscription_* 配置仍可用
#    - buildEmbeddedUrl() 参数链路不被破坏
#    - update_service.go 仍指向 pigzwy/sub2api

# 4. 如 ent/schema 或 wire 生成源被冲突处理改动，再重跑生成
cd backend
go generate ./ent
go generate ./cmd/server
cd ..

# 5. 核对 fork 专属补丁是否还在
grep -n 'githubRepo' backend/internal/service/update_service.go
# 如果变回 Wei-Shaw，手动改回 pigzwy 并 commit

# 6. 跑第 8 节的验证清单

# 7. 全部通过后，合回 main
git checkout main
git merge --ff-only merge/upstream-vX.Y.Z
```

**方案 A 的注意事项：**

- 本方案默认**不**因为 payment 文件名命中黑名单就整块 revert / 删除
- payment migration 默认跟随已合入代码一起保留，除非你已经明确证明某个 SQL 与当前保留代码无关且会破坏外挂支付链路
- 判断标准始终是：外挂支付入口是否仍可用、程序能否正常启动、fork 专属补丁是否仍在

### 7.2 方案 B：仅回滚“接管外挂支付入口”的点位

**适用条件：**
- 方案 A 合并后，upstream payment 改动已经替换了 `/purchase`、删掉了 `purchase_subscription_*`、破坏了 `buildEmbeddedUrl()` 链路
- 或程序因局部 payment 接管点而无法启动，但没有必要回滚整套 payment 模块

**执行步骤：**

```bash
git checkout -b fix/upstream-payment-entry-restore

# 只恢复 fork 明确依赖的入口与配置：
# - /purchase 路由
# - PurchaseSubscriptionView.vue
# - purchase_subscription_* DTO / 设置项
# - embedded-url.ts 参数构造
#
# 不主动删除 backend/internal/payment/**、payment migration、
# admin payment config 或其他已合入但未接管主入口的代码
```

**方案 B 的取舍：**
- 好处：精准可控，只修真正影响外挂支付的接管点
- 代价：
  - 需要人工判断哪些文件只是“payment 代码存在”，哪些文件是真的“接管了主链路”
  - 如果误删 migration / schema，容易再次出现“程序根本起不来”的问题

### 7.3 何时用 A，何时用 B

| 情况 | 推荐方案 |
|---|---|
| 文本无冲突，且 merge 后仍能保住外挂支付入口 | **A** |
| 文本有冲突，但冲突集中在路由 / 设置 / 页面入口 | **A**（人工保入口） |
| 合并后 `/purchase` 被接管、设置项被删、iframe 链路断掉 | **B** |
| 程序启动失败且能定位到是局部接管点造成 | **B** |
| payment 代码已合入但未影响外挂支付 | **A**，不要额外清理 |

---

## 8. 合并后验证清单

按顺序执行，每一项都要通过。**任何一项失败都不能合回 main。**

### 8.1 Fork 专属改动核对

```bash
# update_service.go 的 githubRepo 应为 pigzwy/sub2api
grep 'githubRepo' backend/internal/service/update_service.go

# Fork 专属文件都应存在
ls .github/workflows/fork-docker-build.yml AI-CLI-Guide.md FORK_DEV_GUIDE.md FORK_UPSTREAM_MERGE.md
```

### 8.2 后端构建 / 测试

```bash
cd backend

# 生成代码（如合并中处理了 ent/schema 或 wire 生成源，则重跑）
go generate ./ent
go generate ./cmd/server

# 编译
go build -tags embed ./cmd/server

# 单元测试
go test -tags=unit ./...

# Lint（可选）
golangci-lint run ./...
```

### 8.3 前端构建

```bash
cd frontend
pnpm install  # 如果 package.json 有动
pnpm build
pnpm test     # 如果有前端测试
```

### 8.4 自定义支付回归（**本 Fork 特有，必检**）

要确认外部支付 iframe 嵌入仍然正常工作：

1. **后端 DTO**：
   ```bash
   grep -n 'PurchaseSubscription' backend/internal/handler/dto/settings.go
   # 必须仍然有 PurchaseSubscriptionEnabled / PurchaseSubscriptionURL
   ```

2. **前端路由**：
   ```bash
   grep -n 'PurchaseSubscriptionView\|purchase' frontend/src/router/index.ts
   # 必须仍然有 /purchase 路由指向 PurchaseSubscriptionView.vue（方案 A 的情况）
   # 或者有 /custom/:id 指向 CustomPageView.vue（方案 B 或全接受未来版本的情况）
   ```

3. **Admin 设置页**：
   ```bash
   grep -n 'purchase_subscription' frontend/src/views/admin/SettingsView.vue
   # 方案 A：应该还有开关
   ```

4. **embedded-url.ts 未被破坏**：
   ```bash
   grep -n 'buildEmbeddedUrl' frontend/src/utils/embedded-url.ts
   ```

5. **启动服务本地手动验证**（最可靠）：
   - 起服务 → 登录 → 配置一个 `purchase_subscription_url` 指向测试页
   - 访问 "购买订阅" 菜单 → 检查 iframe 是否带上了 `user_id`、`token`、`theme`、`ui_mode=embedded` 这些 query 参数
   - 在外部页面里尝试读取这些参数，验证集成逻辑没断

### 8.5 数据库迁移

如果本次合并带进了新的 migration SQL：

```bash
ls backend/migrations/ | tail -20
# 记录新增了哪些 migration，评估对生产库的影响
# 特别警惕 *purchase* 或 *payment* 命名的 migration
```

**已知的迁移陷阱**：upstream `backend/migrations/098_migrate_purchase_subscription_to_custom_menu.sql` 会自动把 `purchase_subscription_url` 搬到 `custom_menu_items`。方案 A 的 revert 通常会把这个文件也 revert 掉。如果合并后它仍然存在，要评估是否要手动删掉，避免生产环境运行时执行它。

#### 8.5.1 2026-04 实战坑：机械清理 payment migration，会导致已合入代码缺表而起不来

本 Fork 在一次升级到 `v0.1.115` 的实战中踩过下面这个坑：

- codex 在合并时把 payment 相关代码带进来了
- 后续又按旧文档思路，把几条 payment migration 当成黑名单清掉
- 结果程序启动时发现代码已经引用 payment 相关表，但数据库迁移没跟上，于是直接起不来
- 最后把下面这些 SQL 补回去后，程序才能恢复启动：
  - `111_payment_routing_and_scheduler_flags.sql`
  - `112_add_payment_order_provider_key_snapshot.sql`
  - `117_add_payment_order_provider_snapshot.sql`
  - `119_enforce_payment_orders_out_trade_no_unique.sql`
  - `120_enforce_payment_orders_out_trade_no_unique_notx.sql`
  - `120a_align_payment_orders_out_trade_no_index_name.sql`
- 启动时先报：
  - `relation "payment_orders" does not exist`
- 之后又因为人工往 `schema_migrations` 里补了**错误 checksum**，进一步演变成：
  - `migration ... checksum mismatch (db=... file=...)`

**根因**：

1. 合并策略和清理策略不一致：代码已保留，migration 却被删掉
2. payment migration 被按“文件名命中黑名单”机械处理，没和实际保留的代码一起评估
3. 应急处理时把这些 migration 手工记入了 `schema_migrations`
4. 但使用了**错误的 checksum 计算方式**

**重要**：迁移器校验的不是 shell 里 `sha256sum <file>` 的原始文件哈希，而是 Go 代码中的：

```go
checksum = sha256(strings.TrimSpace(fileContent))
```

也就是说：

- 不能直接拿原始文件 `sha256sum` 结果写进 `schema_migrations`
- 最稳妥的应急方式是：
  - 直接用启动日志里 `file=...` 后面的值
  - 或按迁移器逻辑对 `TrimSpace(content)` 后再算 SHA256

**合并阶段的正确做法**：

- 先判断当前是否准备保留 upstream payment 相关代码；只要代码保留了，就**不能**按文件名机械删除对应 migration
- payment migration 必须和保留中的代码一起评估；重点看它们是否被运行时查询、服务初始化、路由或后台配置依赖
- 不要把“payment 代码合进来了，但 payment migration 被删了”这种半残状态带上生产
- 当前策略下，默认优先保留 migration，除非已经明确证明某条 SQL 对现有保留代码无依赖

**线上止血顺序**：

1. 先看日志，确认卡在哪个 migration 文件
2. 如果是 payment 相关缺表：
   - 先检查是不是迁移被误删、误跳过，或代码与 schema 状态不一致
   - 优先恢复与当前保留代码匹配的 migration / schema，再重建镜像
   - 如果必须现场止血，再手工修 `schema_migrations`
3. 手工修 `schema_migrations` 时：
   - `checksum` 必须填日志里的 `file=...` 值，或用与迁移器一致的算法重算
   - 不要用原始文件 `sha256sum`
4. 每修一条后重启服务，看下一条

**经验结论**：

- payment migration 不能再被视为“命中 payment 就删”的自动清理对象
- 如果 payment 代码已经保留，migration 通常也要一并保留，至少先保证程序能启动
- `schema_migrations` 的应急补录有风险，只有在明确知道当前镜像对应 checksum 的情况下才允许做
- 以后只要线上日志出现：
  - `relation "payment_orders" does not exist`
  - 或 `migration ... checksum mismatch`
  就先检查是不是 payment migration 被误删、误跳过，或代码与 migration 状态不一致

#### 8.5.2 当前仓库 payment 保留清单（v0.1.115 基线）

下次合并前，先按这份清单判断哪些是 **必须保住的外挂支付入口**，哪些是 **已经合入但可静置保留的上游 payment 代码**，不要再凭文件名直接删。

**A. 必须保住的外挂支付入口**

- 路由 `/purchase` 仍指向 `frontend/src/views/user/PurchaseSubscriptionView.vue`
- `frontend/src/views/user/PurchaseSubscriptionView.vue`
- `frontend/src/utils/embedded-url.ts`
- `backend/internal/handler/dto/settings.go` 中的 `purchase_subscription_enabled` / `purchase_subscription_url`
- `backend/internal/handler/admin/setting_handler.go` 对 `purchase_subscription_*` 的读写
- `backend/internal/service/domain_constants.go` 中 `SettingKeyPurchaseSubscriptionEnabled` / `SettingKeyPurchaseSubscriptionURL`
- `frontend/src/views/admin/SettingsView.vue` 中 `purchase_subscription_*` 设置项
- `frontend/src/components/layout/AppSidebar.vue` 中“购买订阅”菜单入口
- `frontend/src/types/index.ts`、`frontend/src/stores/app.ts`、`frontend/src/api/admin/settings.ts` 里对 `purchase_subscription_*` 的定义

**处理规则：**

- 这些文件如果在 merge 后被 payment v2 改到，优先恢复 fork 的外挂支付入口语义
- 判断标准不是“文件是不是 payment 相关”，而是 `/purchase`、iframe 参数链路、admin 配置和外部页接入能力是否还在

**B. 当前已合入但默认允许静置保留的上游 payment 代码**

- `backend/internal/server/routes/payment.go`
- `backend/internal/payment/**`
- `backend/internal/service/payment_*`
- `backend/ent/payment*`
- `backend/ent/schema/payment_*.go`
- `backend/ent/schema/subscription_plan.go`
- `frontend/src/components/payment/**`
- `frontend/src/stores/payment.ts`
- `frontend/src/types/payment.ts`
- `frontend/src/api/payment.ts`
- `frontend/src/views/admin/orders/**`
- payment 相关 migration，包括：
  - `111_payment_routing_and_scheduler_flags.sql`
  - `112_add_payment_order_provider_key_snapshot.sql`
  - `117_add_payment_order_provider_snapshot.sql`
  - `119_enforce_payment_orders_out_trade_no_unique.sql`
  - `120_enforce_payment_orders_out_trade_no_unique_notx.sql`
  - `120a_align_payment_orders_out_trade_no_index_name.sql`

**处理规则：**

- 只要这些代码没有替换外挂支付入口、没有导致程序无法启动，就默认先保留
- 不要为了“和文档黑名单一致”去额外删这些文件
- 特别是 migration，必须与已保留代码一起评估，不能单独清理

**C. 下次合并时的高风险检查点**

- `frontend/src/router/index.ts`：
  看 `/purchase` 是否被改成 `PaymentView.vue` 或其他内置支付入口
- `frontend/src/views/user/PurchaseSubscriptionView.vue`：
  看 iframe 是否还在，是否仍调用 `buildEmbeddedUrl()`
- `frontend/src/utils/embedded-url.ts`：
  看 `user_id`、`token`、`theme`、`lang`、`ui_mode=embedded`、`src_host`、`src_url` 是否还在
- `backend/internal/handler/dto/settings.go`、`frontend/src/views/admin/SettingsView.vue`：
  看 `purchase_subscription_enabled` / `purchase_subscription_url` 是否被删或被别的 payment 配置替代
- `backend/cmd/server/VERSION`：
  每次合并 release 后同步核对版本号
- `backend/internal/service/update_service.go`：
  确保 `githubRepo` 仍是 `pigzwy/sub2api`

**D. 当前仓库里已知的 payment 保留状态**

- `frontend/src/views/user/PaymentView.vue` 可能存在，这是上游内置支付页；存在本身不等于接管 fork 主入口
- `frontend/src/views/user/__tests__/PaymentView.spec.ts`、`frontend/src/views/user/__tests__/PaymentResultView.spec.ts`、`frontend/src/views/user/paymentWechatResume.ts` 可能仍依赖 `@/types/payment`
- 合并判断以路由和菜单入口为准：`/purchase` 必须仍指向 `PurchaseSubscriptionView.vue`，不能被改成 `PaymentView.vue`

**处理规则：**

- 这些属于“已合入上游 payment 痕迹”，不是本次必须清理项
- 只有在下次合并或验证时，它们实际导致构建 / 测试失败，才单独处理

#### 8.5.3 2026-04-25 合并 v0.1.118 记录

本次从本地 `0.1.117` 合并到 upstream `0.1.118`，采用方案 A：直接 merge upstream，保留上游非入口型改动，重点守住 fork 外挂支付入口。

合并后已核对的点：

- `backend/internal/service/update_service.go` 仍为 `githubRepo = "pigzwy/sub2api"`
- fork 专属文件仍存在：`.github/workflows/fork-docker-build.yml`、`AI-CLI-Guide.md`、`FORK_DEV_GUIDE.md`、`FORK_UPSTREAM_MERGE.md`
- `/purchase` 仍指向 `frontend/src/views/user/PurchaseSubscriptionView.vue`
- `/custom/:id` 仍指向 `frontend/src/views/user/CustomPageView.vue`
- `frontend/src/utils/embedded-url.ts` 仍传入 `user_id`、`token`、`theme`、`lang`、`ui_mode=embedded`、`src_host`、`src_url`
- `purchase_subscription_enabled` / `purchase_subscription_url` 仍保留在 DTO、admin setting handler、domain constants、SettingsView、public settings 类型中
- `backend/internal/service/payment_order_expiry_service.go` 仍保留 `payment_orders` 缺表探测，缺表时禁用过期订单后台任务
- 本次没有引入 `098_migrate_purchase_subscription_to_custom_menu.sql`
- 本次新增 upstream migration 为 `130_add_user_affiliates.sql`、`131_affiliate_rebate_hardening.sql`

本次命中的支付相关上游 commit：

- `8f28a834 fix(payment): 同时启用易支付和 Stripe 时显示 Stripe 按钮`

判断：该 commit 只改 payment 组件测试、`paymentFlow.ts` 和 `PaymentView.vue`，没有接管 `/purchase` 或 `purchase_subscription_*`，按当前策略允许静置保留。

---

## 9. 回滚预案

### 9.1 合并过程中出错

因为你始终在 `merge/upstream-vX.Y.Z` 分支上操作，main 从没动过：

```bash
git checkout main
git branch -D merge/upstream-vX.Y.Z     # 直接扔掉整条失败的合并
# 或先保留备查
git branch -m merge/upstream-vX.Y.Z merge/upstream-vX.Y.Z-failed
```

### 9.2 已经合回 main 才发现问题

```bash
# 找到合回前的 commit
git reflog | head -20
# 比如合并前 HEAD 是 aef27d10
git reset --hard aef27d10    # ⚠️ 如果已经 push，要和团队沟通
```

**⚠️ 注意**：如果已经 push 到 `origin/main`，`reset --hard` 需要 force push，按用户的全局规则 AI 不能直接跑。这种情况下更安全的做法是写一个 revert commit 把合并 revert 掉：

```bash
git revert -m 1 <合并 commit>
```

### 9.3 生产环境已经部署后才发现问题

- 立即切回上一个稳定镜像 tag
- 不要尝试热修复

---

## 10. 维护本文档

以下情况要更新本文档：

- [ ] upstream 又合并了新的支付相关 PR → 追加到 **3.2.3 节**的 PR 表格
- [ ] Fork 又新增了对 upstream 文件的补丁 → 追加到 **2.2 节**的表格
- [ ] 发现新的黑名单文件/目录模式 → 追加到 **3.2.1 节**
- [ ] 某次合并踩到新坑 → 记录到 **8 节**或 **9 节**
- [ ] 确认某个灰色地带 feature 走向 → 升级到 **3 节**（拒绝）或降级移出 **5 节**（全接）

---

## 附录 A：本 Fork 的"自写支付"架构速记

供 AI 快速理解上下文。

- **外部支付页**：单独一个 web 应用（不在本仓库），用户自己写的
- **嵌入方式**：本项目的 `PurchaseSubscriptionView.vue` 用 iframe 嵌入那个外部页
- **会话传递**：通过 query 参数传 `user_id`、`token`、`theme`、`lang`、`ui_mode=embedded`、`src_host`、`src_url`
- **URL 配置**：在 admin 设置里配 `purchase_subscription_enabled`、`purchase_subscription_url`
- **菜单入口**：前端侧边栏"购买订阅"菜单项，对应路由 `/purchase`
- **外部页认证**：通过全局 `admin_api_key`（`x-api-key` header）调用管理员接口，属于“外部系统集成”能力
- **外部页回调**：外部支付完成后调用 `/api/v1/admin/redeem-codes/create-and-redeem`
- **到账模型**：外部系统用外部订单号映射成固定 redeem code，再由后端立即兑换；余额/订阅变更仍然走本仓库现有 `RedeemService`

上游的内置支付系统（`PaymentView.vue` + Stripe/Alipay/WxPay/EasyPay provider + `payment_order` 表）**仍然是另一套东西**。本 Fork 继续拒绝它接管主入口和主充值链路；但若相关代码已合入且未影响外挂支付、程序也能正常启动，则可先静置保留，不做机械清理。

---

## 附录 B：快速命令速查

```bash
# 查看当前合并范围
git log --oneline $(git merge-base HEAD upstream/main)..upstream/main

# 查看某个 commit 改了哪些文件
git show --stat <sha>

# 查看 upstream 的某个文件（不污染工作区）
git show upstream/main:<path>

# 验证文本无冲突
git merge-tree --write-tree HEAD upstream/main  # 只出一行 tree SHA 即无冲突

# 撤销合并 commit
git revert -m 1 <merge-sha>

# 撤销普通 commit
git revert <sha>

# 只拿一个 commit
git cherry-pick <sha>
```
