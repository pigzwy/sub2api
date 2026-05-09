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
