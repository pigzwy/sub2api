package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestSiteStatusSQLReadsRawLogsOnly(t *testing.T) {
	usage := strings.ToLower(siteStatusUsageSQL)
	errs := strings.ToLower(siteStatusErrorSQL)

	// 直接统计请求日志与错误日志，不读渠道监控的任何表
	require.Contains(t, usage, "from usage_logs ul")
	require.Contains(t, errs, "from ops_error_logs el")
	for _, q := range []string{usage, errs} {
		require.NotContains(t, q, "channel_monitor")
		require.Contains(t, q, "group_id = any($4)")
		require.Contains(t, q, "created_at >= $2")
		require.Contains(t, q, "created_at < $3")
		require.Contains(t, q, "grouping sets ((group_id, bucket_start), (group_id))")
		require.Contains(t, q, "timestamptz '1970-01-01 00:00:00+00'", "桶对齐到固定 UTC 原点，不随会话时区变化")
	}

	// 成功：与仓库通用口径一致
	require.Contains(t, usage, "ul.actual_cost > 0")
	require.Contains(t, usage, "coalesce(ul.request_type, 0) not in (4, 6)")
	require.Contains(t, usage, "percentile_cont(0.5)")

	// 失败：只算服务侧、用户确实遇到的失败
	require.Contains(t, errs, "el.error_owner in ('provider', 'platform')")
	require.Contains(t, errs, "not coalesce(el.is_business_limited, false)")
	require.Contains(t, errs, "not coalesce(el.is_count_tokens, false)")
	require.Contains(t, errs, "coalesce(el.status_code, 0) >= 500 or coalesce(el.status_code, 0) = 429")
}

func TestSiteStatusRepositoryGroupStatsMergesUsageAndErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := NewSiteStatusRepository(db)

	end := time.Date(2026, 11, 27, 4, 30, 0, 0, time.UTC)
	start := end.Add(-24 * time.Hour)
	b0 := start
	b1 := start.Add(30 * time.Minute)
	ids := []int64{43, 71}

	mock.ExpectQuery(`FROM usage_logs ul`).
		WithArgs("1800 seconds", start, end, pq.Array(ids)).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "bucket_start", "count", "ttft"}).
			AddRow(int64(43), b0, int64(98), 1799.6).
			AddRow(int64(43), nil, int64(4990), 1800.2).
			AddRow(int64(71), b1, int64(7), nil))
	mock.ExpectQuery(`FROM ops_error_logs el`).
		WithArgs("1800 seconds", start, end, pq.Array(ids)).
		WillReturnRows(sqlmock.NewRows([]string{"group_id", "bucket_start", "count"}).
			AddRow(int64(43), b0, int64(2)).
			AddRow(int64(43), nil, int64(10)).
			// 只有失败、没有任何成功的桶：也要有一行
			AddRow(int64(71), b0, int64(5)))

	stats, err := repo.GroupStats(context.Background(), ids, start, end, 30*time.Minute)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	find := func(groupID int64, total bool, bucket time.Time) service.SiteStatusStat {
		for _, st := range stats {
			if st.GroupID == groupID && st.Total == total && (total || st.BucketStart.Equal(bucket)) {
				return st
			}
		}
		t.Fatalf("missing stat group=%d total=%v bucket=%s", groupID, total, bucket)
		return service.SiteStatusStat{}
	}
	require.Len(t, stats, 4)

	g43b0 := find(43, false, b0)
	require.Equal(t, int64(98), g43b0.Success)
	require.Equal(t, int64(2), g43b0.Errors)
	require.Equal(t, int64(1800), *g43b0.TTFTP50Ms, "中位数四舍五入到毫秒")

	g43total := find(43, true, time.Time{})
	require.Equal(t, int64(4990), g43total.Success)
	require.Equal(t, int64(10), g43total.Errors)

	g71b1 := find(71, false, b1)
	require.Equal(t, int64(7), g71b1.Success)
	require.Zero(t, g71b1.Errors)
	require.Nil(t, g71b1.TTFTP50Ms)

	g71b0 := find(71, false, b0)
	require.Zero(t, g71b0.Success)
	require.Equal(t, int64(5), g71b0.Errors)
}

func TestSiteStatusRepositoryGroupStatsSkipsQueryWithoutGroups(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	end := time.Date(2026, 11, 27, 4, 30, 0, 0, time.UTC)
	stats, err := NewSiteStatusRepository(db).GroupStats(context.Background(), nil, end.Add(-time.Hour), end, 30*time.Minute)
	require.NoError(t, err)
	require.Empty(t, stats)
	require.NoError(t, mock.ExpectationsWereMet(), "没有公开分组时不该发任何查询")
}
