package repository

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type siteStatusRepository struct {
	db *sql.DB
}

// NewSiteStatusRepository 创建官网状态页的统计仓库。
func NewSiteStatusRepository(db *sql.DB) service.SiteStatusRepository {
	return &siteStatusRepository{db: db}
}

// 桶对齐到固定的 UTC 原点，与会话时区无关。
const siteStatusBinOrigin = "TIMESTAMPTZ '1970-01-01 00:00:00+00'"

// siteStatusUsageSQL 统计成功请求数与首字延迟中位数。
//
// usage_logs 没有成功标记列，沿用仓库里通用的判定：成功落账的请求 actual_cost > 0
// （见 usageLogSuccessFilterUL）。request_type 4 和 6 是被上游安全策略拦下但仍计费的请求，不算成功。
// GROUPING SETS 一次扫描同时得到「每桶」和「整个窗口」两组结果，bucket_start 为 NULL 的是窗口汇总。
const siteStatusUsageSQL = `
WITH s AS (
  SELECT ul.group_id,
         date_bin($1::interval, ul.created_at, ` + siteStatusBinOrigin + `) AS bucket_start,
         COALESCE(NULLIF(ul.request_id, ''), 'usage:' || ul.id::text) AS request_key,
         ul.first_token_ms
  FROM usage_logs ul
  WHERE ul.created_at >= $2 AND ul.created_at < $3
    AND ul.group_id = ANY($4)
    AND ul.actual_cost > 0
    AND COALESCE(ul.request_type, 0) NOT IN (4, 6)
)
SELECT group_id, bucket_start,
       COUNT(DISTINCT request_key),
       percentile_cont(0.5) WITHIN GROUP (ORDER BY first_token_ms)
         FILTER (WHERE first_token_ms IS NOT NULL AND first_token_ms >= 0)
FROM s
GROUP BY GROUPING SETS ((group_id, bucket_start), (group_id))`

// siteStatusErrorSQL 统计服务侧失败的请求数。只算用户确实遇到、且责任不在用户的失败：
//   - error_owner 为 provider 或 platform：上游或网关自身的问题。client（参数错误、鉴权失败）不算；
//   - 非业务限制：余额不足、订阅失效、用户级并发/限额等不算；
//   - 最终状态码 >= 500，或 429（上游限流、容量不足，换号后仍失败）。
//     上游对错误请求返回的 400/404/413 等是请求本身的问题，不算；499 是客户端主动断开，不算；
//   - count_tokens 探测请求不算。
//
// 同一请求可能留下多条记录，按 request_id 去重。
const siteStatusErrorSQL = `
WITH e AS (
  SELECT el.group_id,
         date_bin($1::interval, el.created_at, ` + siteStatusBinOrigin + `) AS bucket_start,
         COALESCE(NULLIF(el.request_id, ''), 'error:' || el.id::text) AS request_key
  FROM ops_error_logs el
  WHERE el.created_at >= $2 AND el.created_at < $3
    AND el.group_id = ANY($4)
    AND NOT COALESCE(el.is_count_tokens, false)
    AND NOT COALESCE(el.is_business_limited, false)
    AND el.error_owner IN ('provider', 'platform')
    AND (COALESCE(el.status_code, 0) >= 500 OR COALESCE(el.status_code, 0) = 429)
)
SELECT group_id, bucket_start, COUNT(DISTINCT request_key)
FROM e
GROUP BY GROUPING SETS ((group_id, bucket_start), (group_id))`

type siteStatusKey struct {
	groupID int64
	total   bool
	bucket  int64
}

func (r *siteStatusRepository) GroupStats(ctx context.Context, groupIDs []int64, start, end time.Time, bucket time.Duration) ([]service.SiteStatusStat, error) {
	if len(groupIDs) == 0 || !end.After(start) || bucket <= 0 {
		return nil, nil
	}
	interval := fmt.Sprintf("%d seconds", int(bucket.Seconds()))
	args := []any{interval, start.UTC(), end.UTC(), pq.Array(groupIDs)}

	stats := make(map[siteStatusKey]*service.SiteStatusStat)
	order := make([]siteStatusKey, 0, len(groupIDs)*8)
	at := func(groupID int64, bucketStart sql.NullTime) *service.SiteStatusStat {
		key := siteStatusKey{groupID: groupID, total: !bucketStart.Valid}
		if bucketStart.Valid {
			key.bucket = bucketStart.Time.Unix()
		}
		if st := stats[key]; st != nil {
			return st
		}
		st := &service.SiteStatusStat{GroupID: groupID, Total: key.total}
		if bucketStart.Valid {
			st.BucketStart = bucketStart.Time.UTC()
		}
		stats[key] = st
		order = append(order, key)
		return st
	}

	usageRows, err := r.db.QueryContext(ctx, siteStatusUsageSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("site status usage stats: %w", err)
	}
	defer func() { _ = usageRows.Close() }()
	for usageRows.Next() {
		var (
			groupID     int64
			bucketStart sql.NullTime
			success     int64
			ttft        sql.NullFloat64
		)
		if err := usageRows.Scan(&groupID, &bucketStart, &success, &ttft); err != nil {
			return nil, fmt.Errorf("scan site status usage stats: %w", err)
		}
		st := at(groupID, bucketStart)
		st.Success = success
		if ttft.Valid && !math.IsNaN(ttft.Float64) && ttft.Float64 >= 0 {
			ms := int64(math.Round(ttft.Float64))
			st.TTFTP50Ms = &ms
		}
	}
	if err := usageRows.Err(); err != nil {
		return nil, fmt.Errorf("site status usage stats: %w", err)
	}

	errorRows, err := r.db.QueryContext(ctx, siteStatusErrorSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("site status error stats: %w", err)
	}
	defer func() { _ = errorRows.Close() }()
	for errorRows.Next() {
		var (
			groupID     int64
			bucketStart sql.NullTime
			errorsCount int64
		)
		if err := errorRows.Scan(&groupID, &bucketStart, &errorsCount); err != nil {
			return nil, fmt.Errorf("scan site status error stats: %w", err)
		}
		at(groupID, bucketStart).Errors = errorsCount
	}
	if err := errorRows.Err(); err != nil {
		return nil, fmt.Errorf("site status error stats: %w", err)
	}

	out := make([]service.SiteStatusStat, 0, len(order))
	for _, key := range order {
		out = append(out, *stats[key])
	}
	return out, nil
}
