package service

import (
	"context"
	"time"
)

// SiteStatusStat 是某个分组在一个时间桶内（或整个查询窗口内）的请求统计，
// 直接来自请求日志与错误日志，不经过渠道监控。
type SiteStatusStat struct {
	GroupID int64
	// BucketStart 是桶的起点；Total 为 true 时这一行是整个查询窗口的汇总，BucketStart 无意义。
	BucketStart time.Time
	Total       bool
	// Success 是成功完成的请求数。
	Success int64
	// Errors 是服务侧失败的请求数（定义见 repository.siteStatusErrorSQL）。
	Errors int64
	// TTFTP50Ms 是成功请求首字延迟的中位数；没有样本时为 nil。
	TTFTP50Ms *int64
}

// SiteStatusRepository 为官网状态页统计各分组的请求成败。
type SiteStatusRepository interface {
	// GroupStats 返回 groupIDs 在 [start, end) 内按 bucket 切分的统计，外加每个分组一行窗口汇总。
	// 没有任何请求的桶不返回行。
	GroupStats(ctx context.Context, groupIDs []int64, start, end time.Time, bucket time.Duration) ([]SiteStatusStat, error)
}
