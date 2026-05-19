# 合并记录

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
