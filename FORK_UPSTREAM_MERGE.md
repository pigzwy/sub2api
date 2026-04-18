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
2. **拒绝上游的内置支付系统**（本 Fork 使用自写的外部支付页 + iframe 嵌入，见第 3 节详解）
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

**本 Fork 不要这个系统**，原因：
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
- **因此长期策略不变**：
  - 继续拒绝 upstream 内置支付系统主体
  - 继续保留 fork 的外部支付为唯一主充值链路
  - upstream 在共享文件里的非支付优化仍然要接；必要时人工解冲突或手工移植，但**不能让 payment v2 接管 `/purchase`、`purchase_subscription_*` 配置和当前充值主链路**

### 3.2 黑名单 — 命中即跳过

#### 3.2.1 文件 glob 模式（任一命中即认为是支付相关）

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

**以后遇到新的支付相关 PR 要追加到本表，AI 每次合并前读本表就知道要跳过哪些。**

**注意**：如果黑名单 PR 改的文件在 fork 里已经被删除，合并时会出现 `modify/delete` 冲突。此时**直接 `git rm` 这些支付文件**比 `git revert -m 1 <PR>` 更干净（revert 会把文件拉回来，还得再删一次）。v0.1.112 合并时 PR #1610 和 #1612 就是这样处理的。

### 3.3 黑名单的边界：已被 upstream 移除的"旧 iframe 入口"

upstream 在 v0.1.111 已经**删除**了 `frontend/src/views/user/PurchaseSubscriptionView.vue`，并把路由 `/purchase` 改成指向新的 `PaymentView.vue`。我们的处理方式是：

- 如果选**方案 A**（merge + revert 黑名单 PR）：revert 后这些会被还原，旧 iframe 入口继续可用
- 如果选**方案 B**（cherry-pick）：不合支付 PR 就等于不删除，也继续可用
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
4. **默认接受**，除非 merge + revert 后构建直接失败才需要特别处理

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

### 7.1 方案 A：merge + revert 黑名单 PR（推荐）

**适用条件：**
- Step 3 显示文本层无冲突
- 黑名单 PR 都是明确的 merge commit（有 `-m 1` 可用）

**执行步骤：**

```bash
# 1. 开安全分支，绝不动 main
git checkout -b merge/upstream-vX.Y.Z

# 2. 合并 upstream（verify 过无冲突才跑这步）
git merge --no-ff upstream/main \
  -m "Merge upstream vX.Y.Z (payment system excluded via revert)"

# 3. 按照分析报告列出的黑名单 PR，从新到旧 revert
#    （例如 v0.1.111 的情况）
git revert -m 1 54490cf6  # 先撤较新的 feat/payment-docs
git revert -m 1 97f14b7a  # 再撤较早的 feat/payment-system-v2

# 4. 处理 revert 衍生问题（ent/wire 生成文件）
cd backend
go generate ./ent
go generate ./cmd/server
cd ..

# 5. 把生成文件的改动 stage 上来补到最后一个 revert commit 里
#    （或单独一个 "fix(gen): regenerate after payment revert" commit）
git add backend/ent backend/cmd/server/wire_gen.go
git commit -m "chore(gen): regenerate ent/wire after reverting payment PRs"

# 6. 核对 fork 专属补丁是否还在
grep -n 'githubRepo' backend/internal/service/update_service.go
# 如果变回 Wei-Shaw，手动改回 pigzwy 并 commit

# 7. 跑第 8 节的验证清单

# 8. 全部通过后，合回 main
git checkout main
git merge --ff-only merge/upstream-vX.Y.Z
```

**方案 A 的 revert 命令注意事项：**

- **必须加 `-m 1`**：因为 revert 的对象是 merge commit，`-m 1` 表示把第一个父分支（主线）作为参照面。
- revert 顺序：**从新到旧**（先 54490cf6 后 97f14b7a）。如果倒过来，前一个 PR 依赖了后一个 PR 的文件会冲突。
- revert 之后要重跑 `go generate`，因为 upstream 的 payment PR 改了 `ent/schema/user.go`（加了支付相关字段），revert 会把 schema 改回去，但生成文件也要跟着回退到一致状态。

### 7.2 方案 B：只 cherry-pick 关键 commit

**适用条件：**
- 方案 A 的 revert 过程出现不可调和的冲突
- 或本次 upstream 更新量大、且大部分是支付相关，值得拿的非支付 commit 只有少数几个

**执行步骤：**

```bash
git checkout -b fix/upstream-critical-picks

# 按时间顺序从老到新 cherry-pick 白名单里的关键 commit
# 例如：
git cherry-pick ce833d91    # CSP frame-src
git cherry-pick 6401dd7c    # 错误日志 body 上限
git cherry-pick d8fa38d5    # 账号状态筛选
git cherry-pick 118ff85f    # LoadFactor 同步
git cherry-pick b6bc0423    # axios 前端
git cherry-pick 217b7ea6    # axios 全量
git cherry-pick cb016ad8    # Anthropic 400
git cherry-pick 9648c432    # API client 类型

# 遇冲突要么手动解，要么 --skip 跳过无关紧要的
# 跑验证
# 合回 main
```

**方案 B 的取舍：**
- 好处：精准可控，完全避开支付代码
- 代价：
  - 漏掉灰色地带的大 feature（OIDC、表格后端排序等）
  - 单个大 feature 由若干 commit 组成时要按依赖顺序全部 cherry-pick，工作量大
  - 长期来看每次合并都用 B 方案，上游累积改动会越来越难跟

### 7.3 何时用 A，何时用 B

| 情况 | 推荐方案 |
|---|---|
| 文本无冲突 + 黑名单 PR 清晰 | **A** |
| 文本有冲突，但冲突都在非支付文件 | **A**（人工解冲突后继续） |
| 本次 upstream 全是支付 PR，没啥好拿的 | **不合** 或 **B** |
| Revert 后 ent/wire 生成文件一直不一致 | 从 A 退到 **B** |
| 黑名单 PR 和白名单 commit 已经交织难分 | **B** |

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

# 生成代码（revert 后必须重跑）
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

上游的内置支付系统（`PaymentView.vue` + Stripe/Alipay/WxPay/EasyPay provider + `payment_order` 表）**完全是另一套东西**，跟这套 iframe 嵌入方案没有共存价值，所以一律拒绝。

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
