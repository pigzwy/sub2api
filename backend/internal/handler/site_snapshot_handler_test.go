package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubSitePlazaLister struct {
	groups []service.PlazaGroup
	err    error
}

func (s *stubSitePlazaLister) ListGroups(context.Context) ([]service.PlazaGroup, error) {
	return s.groups, s.err
}

type stubSiteStatusRepo struct {
	stats []service.SiteStatusStat
	err   error
	delay time.Duration

	calls            atomic.Int64
	mu               sync.Mutex
	lastIDs          []int64
	lastFrom, lastTo time.Time
	lastBucket       time.Duration
	ctxErr           error
}

func (s *stubSiteStatusRepo) GroupStats(ctx context.Context, groupIDs []int64, start, end time.Time, bucket time.Duration) ([]service.SiteStatusStat, error) {
	s.calls.Add(1)
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	s.mu.Lock()
	s.lastIDs, s.lastFrom, s.lastTo, s.lastBucket, s.ctxErr = groupIDs, start, end, bucket, ctx.Err()
	s.mu.Unlock()
	return s.stats, s.err
}

func int64Ptr(v int64) *int64 { return &v }

// siteTestClock 是可以拨动的时钟。
type siteTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *siteTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *siteTestClock) Set(t time.Time) {
	c.mu.Lock()
	c.now = t
	c.mu.Unlock()
}

const siteTestPath = "/api/v1/internal/site/status"

// 04:35:02Z：窗口结束于刚过去的半点 04:30，且已过 5 分钟的日志落稳时间。
var (
	siteTestNow   = time.Date(2026, 11, 27, 4, 35, 2, 0, time.UTC)
	siteTestEnd   = time.Date(2026, 11, 27, 4, 30, 0, 0, time.UTC)
	siteTestStart = siteTestEnd.Add(-24 * time.Hour)
)

func siteTestBucket(groupID int64, index int, success, errorsCount int64, ttft *int64) service.SiteStatusStat {
	return service.SiteStatusStat{
		GroupID: groupID, BucketStart: siteTestStart.Add(time.Duration(index) * 30 * time.Minute),
		Success: success, Errors: errorsCount, TTFTP50Ms: ttft,
	}
}

func siteTestTotal(groupID int64, success, errorsCount int64, ttft *int64) service.SiteStatusStat {
	return service.SiteStatusStat{GroupID: groupID, Total: true, Success: success, Errors: errorsCount, TTFTP50Ms: ttft}
}

func siteTestHandler(groups []service.PlazaGroup, repo *stubSiteStatusRepo, clock *siteTestClock) *SiteSnapshotHandler {
	return &SiteSnapshotHandler{
		plazaService: &stubSitePlazaLister{groups: groups},
		statusRepo:   repo,
		now:          clock.Now,
	}
}

func siteTestGet(t *testing.T, h *SiteSnapshotHandler, target string, headers map[string]string) (*httptest.ResponseRecorder, siteStatusResponse) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range headers {
		c.Request.Header.Set(k, v)
	}
	h.GetStatus(c)
	var resp siteStatusResponse
	if w.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	}
	return w, resp
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestSiteSnapshotHandler_WhitelistSchemaWindowLocalizationAndPrivacy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clock := &siteTestClock{now: siteTestNow}

	groups := []service.PlazaGroup{
		{ID: 43, Platform: "anthropic", Name: "企业 CC-MAX | 限客户端", I18n: service.GroupI18n{"en-US": {Name: "Enterprise CC-MAX | Client Only"}}},
		{ID: 46, Platform: "openai", Name: "企业 Codex | 福利", I18n: service.GroupI18n{"en-US": {Name: "Enterprise Codex | Bonus"}}},
		{ID: 99, Platform: "openai", Name: "专属内部分组", IsExclusive: true},
	}
	repo := &stubSiteStatusRepo{stats: []service.SiteStatusStat{
		siteTestTotal(43, 4990, 10, int64Ptr(1800)),
		siteTestBucket(43, 0, 100, 0, int64Ptr(1750)),
		siteTestBucket(43, 47, 113, 7, int64Ptr(1600)),
		// 专属分组即使仓库误返回了数据，也不得出现在响应里
		siteTestTotal(99, 10, 90, nil),
	}}
	h := siteTestHandler(groups, repo, clock)

	w, resp := siteTestGet(t, h, siteTestPath+"?lang=zh-CN", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "public, max-age=60", w.Header().Get("Cache-Control"))
	require.Contains(t, w.Header().Get("Content-Type"), "application/json")

	// 查询窗口：48 个已结束的半小时桶，结束于刚过去的半点；只查公开分组
	require.Equal(t, siteTestStart, repo.lastFrom)
	require.Equal(t, siteTestEnd, repo.lastTo)
	require.Equal(t, 30*time.Minute, repo.lastBucket)
	require.Equal(t, []int64{43, 46}, repo.lastIDs)

	require.True(t, resp.OK)
	require.Equal(t, 30, resp.BucketMinutes)
	require.Equal(t, siteTestNow.Format(time.RFC3339), resp.UpdatedAt)
	require.Len(t, resp.Groups, 2, "专属分组不得出现")

	g0 := resp.Groups[0]
	require.Equal(t, int64(43), g0.ID)
	require.Equal(t, "anthropic", g0.Platform)
	require.Equal(t, "企业 CC-MAX | 限客户端", g0.Name)
	require.Len(t, g0.Buckets, 48)
	require.Equal(t, siteTestStart.Format(time.RFC3339), g0.Buckets[0].Start)
	require.Equal(t, siteTestStart.Add(30*time.Minute).Format(time.RFC3339), g0.Buckets[0].End)
	require.Equal(t, siteTestEnd.Format(time.RFC3339), g0.Buckets[47].End)
	require.Equal(t, "normal", g0.Buckets[0].Status)
	require.InDelta(t, 100.0, *g0.Buckets[0].Uptime, 0.001)
	require.Equal(t, int64(1750), *g0.Buckets[0].TTFTMs)
	require.Equal(t, "degraded", g0.Buckets[47].Status) // 7/120 ≈ 5.8%
	require.InDelta(t, 94.2, *g0.Buckets[47].Uptime, 0.001)
	require.Equal(t, "degraded", g0.Status, "当前状态取最近一个有流量的桶")
	require.InDelta(t, 99.8, *g0.Uptime24h, 0.001)
	require.Equal(t, int64(1800), *g0.TTFTMs)

	// 完全没有流量的分组照常出现，并且按正常算；没有可报的可用率与首字延迟
	g1 := resp.Groups[1]
	require.Equal(t, int64(46), g1.ID)
	require.Equal(t, "normal", g1.Status)
	require.Nil(t, g1.Uptime24h)
	require.Nil(t, g1.TTFTMs)
	require.Len(t, g1.Buckets, 48)
	for _, b := range g1.Buckets {
		require.Equal(t, "normal", b.Status)
		require.Nil(t, b.Uptime)
		require.Nil(t, b.TTFTMs)
	}

	// 响应里只能有白名单字段：请求数、失败数等绝对数量一律不外泄
	raw := strings.ToLower(w.Body.String())
	for _, forbidden := range []string{"request", "token", "success", "error", "count", "total", "user", "account", "exclusive", "no_data", "专属", "4990", "113"} {
		require.NotContains(t, raw, forbidden, "response must not leak %q", forbidden)
	}
	require.NotContains(t, w.Body.String(), `"code":`)
	require.NotContains(t, w.Body.String(), `"data":`)
	var generic map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &generic))
	require.ElementsMatch(t, []string{"ok", "updated_at", "bucket_minutes", "groups"}, keysOf(generic))
	rawGroups, ok := generic["groups"].([]any)
	require.True(t, ok, "groups must be an array")
	require.NotEmpty(t, rawGroups)
	group0, ok := rawGroups[0].(map[string]any)
	require.True(t, ok, "group must be an object")
	require.ElementsMatch(t, []string{"id", "platform", "name", "status", "uptime_24h", "ttft_ms", "buckets"}, keysOf(group0))
	rawBuckets, ok := group0["buckets"].([]any)
	require.True(t, ok, "buckets must be an array")
	require.NotEmpty(t, rawBuckets)
	bucket0, ok := rawBuckets[0].(map[string]any)
	require.True(t, ok, "bucket must be an object")
	require.ElementsMatch(t, []string{"start", "end", "status", "uptime", "ttft_ms"}, keysOf(bucket0))

	// 英文：分组名换成译文，数据来自同一次计算
	_, en := siteTestGet(t, h, siteTestPath, map[string]string{"Accept-Language": "en-US,en;q=0.9"})
	require.Equal(t, "Enterprise CC-MAX | Client Only", en.Groups[0].Name)
	require.Equal(t, "Enterprise Codex | Bonus", en.Groups[1].Name)
	require.Equal(t, int64(1), repo.calls.Load())
}

func TestSiteStatStatus(t *testing.T) {
	cases := []struct {
		name     string
		success  int64
		errors   int64
		want     string
		observed bool
	}{
		// 样本充足（不少于 20 个请求）
		{"all good", 500, 0, "normal", true},
		{"just under degraded", 96, 4, "normal", true},
		{"at degraded threshold", 95, 5, "degraded", true},
		{"just under outage", 81, 19, "degraded", true},
		{"at outage threshold", 80, 20, "outage", true},
		{"everything failing", 0, 300, "outage", true},
		// 样本不多（5 到 19 个）：最多报波动
		{"few, none failed", 12, 0, "normal", true},
		{"few, one failed is not enough", 11, 1, "normal", true},
		{"few, a fifth failed", 8, 2, "degraded", true},
		{"few, all failed: still not outage", 0, 19, "degraded", true},
		// 少于 5 个：当作没有流量
		{"no traffic", 0, 0, "normal", false},
		{"too few to judge even if all failed", 0, 4, "normal", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, observed := siteStatStatus(service.SiteStatusStat{Success: tc.success, Errors: tc.errors})
			require.Equal(t, tc.want, status)
			require.Equal(t, tc.observed, observed)
		})
	}
}

func TestSiteSnapshotHandler_SlowFirstTokenNeverChangesStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clock := &siteTestClock{now: siteTestNow}
	// 图像、视频分组首字动辄几十秒，这是正常的
	repo := &stubSiteStatusRepo{stats: []service.SiteStatusStat{
		siteTestTotal(71, 900, 0, int64Ptr(48000)),
		siteTestBucket(71, 47, 60, 0, int64Ptr(52000)),
	}}
	_, resp := siteTestGet(t, siteTestHandler([]service.PlazaGroup{{ID: 71, Platform: "composite", Name: "图像视频"}}, repo, clock), siteTestPath, nil)
	require.Equal(t, "normal", resp.Groups[0].Status)
	require.Equal(t, "normal", resp.Groups[0].Buckets[47].Status)
	require.Equal(t, int64(52000), *resp.Groups[0].Buckets[47].TTFTMs)
}

func TestSiteSnapshotHandler_CurrentStatusFollowsLatestBucketWithTraffic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clock := &siteTestClock{now: siteTestNow}

	groups := []service.PlazaGroup{
		{ID: 1, Platform: "openai", Name: "outage then silence"},
		{ID: 2, Platform: "openai", Name: "old incident"},
		{ID: 3, Platform: "openai", Name: "recovered"},
		{ID: 4, Platform: "openai", Name: "diluted by a good day"},
	}
	repo := &stubSiteStatusRepo{stats: []service.SiteStatusStat{
		// 故障后用户不再发请求：最后两个桶没有流量，但不能因此显示正常
		siteTestTotal(1, 4900, 120, nil), siteTestBucket(1, 45, 80, 120, nil),
		// 四小时前出过故障，之后再没流量：早已过去，当前按正常算
		siteTestTotal(2, 4900, 120, nil), siteTestBucket(2, 40, 80, 120, nil),
		// 出过故障但最近一个有流量的桶已经恢复
		siteTestTotal(3, 5000, 120, nil), siteTestBucket(3, 45, 80, 120, nil), siteTestBucket(3, 47, 200, 0, nil),
		// 全天汇总很健康，但最近一个桶在故障：不能被前 23 小时稀释
		siteTestTotal(4, 89640, 360, nil), siteTestBucket(4, 47, 90, 210, nil),
	}}
	_, resp := siteTestGet(t, siteTestHandler(groups, repo, clock), siteTestPath, nil)

	require.Equal(t, "outage", resp.Groups[0].Status)
	require.Equal(t, "normal", resp.Groups[0].Buckets[47].Status, "没有流量的桶本身仍按正常画")
	require.Equal(t, "normal", resp.Groups[1].Status)
	require.Equal(t, "outage", resp.Groups[1].Buckets[40].Status, "历史上的故障仍留在状态条里")
	require.Equal(t, "normal", resp.Groups[2].Status)
	require.Equal(t, "outage", resp.Groups[3].Status)
	require.InDelta(t, 99.6, *resp.Groups[3].Uptime24h, 0.001, "24 小时可用率仍是全天汇总")
}

func TestSiteSnapshotHandler_QuietGroupsFallBackToTheDayTotal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clock := &siteTestClock{now: siteTestNow}

	groups := []service.PlazaGroup{
		{ID: 1, Platform: "anthropic", Name: "quiet and almost always failing"},
		{ID: 2, Platform: "gemini", Name: "a handful of requests, all failed"},
		{ID: 3, Platform: "kimi", Name: "quiet with one unlucky request"},
		{ID: 4, Platform: "openai", Name: "truly no traffic"},
	}
	stats := []service.SiteStatusStat{
		// 全天 122 个请求只成功 5 个，但分散在各个桶里，每个桶都只有两三个
		siteTestTotal(1, 5, 117, nil),
		// 全天 7 个请求全部失败
		siteTestTotal(2, 0, 7, nil),
		// 全天 9 个请求失败 1 个
		siteTestTotal(3, 8, 1, nil),
	}
	for i := 0; i < 48; i += 2 {
		stats = append(stats, siteTestBucket(1, i, 0, 3, nil))
	}
	stats = append(stats, siteTestBucket(2, 47, 0, 2, nil), siteTestBucket(3, 46, 3, 1, nil))
	_, resp := siteTestGet(t, siteTestHandler(groups, &stubSiteStatusRepo{stats: stats}, clock), siteTestPath, nil)

	// 每个桶都“样本不足”，但全天几乎全部失败：实际不可用，不能显示正常
	require.Equal(t, "outage", resp.Groups[0].Status)
	require.InDelta(t, 4.1, *resp.Groups[0].Uptime24h, 0.001)
	// 样本很少：最多报波动
	require.Equal(t, "degraded", resp.Groups[1].Status)
	require.Equal(t, "normal", resp.Groups[2].Status)
	require.Equal(t, "normal", resp.Groups[3].Status)
	require.Nil(t, resp.Groups[3].Uptime24h)

	// 请求太少的桶按正常画，并且不报可用率，避免“正常、可用率 0%”的自相矛盾
	for _, g := range resp.Groups {
		for _, b := range g.Buckets {
			require.Equal(t, "normal", b.Status)
			require.Nil(t, b.Uptime)
		}
	}
}

func TestSiteSnapshotHandler_ComputesAtMostTwicePerWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groups := []service.PlazaGroup{{ID: 1, Platform: "openai", Name: "G", I18n: service.GroupI18n{"en-US": {Name: "G-en"}}}}
	repo := &stubSiteStatusRepo{}

	// 04:31：半点刚过，日志还没落稳
	clock := &siteTestClock{now: time.Date(2026, 11, 27, 4, 31, 0, 0, time.UTC)}
	h := siteTestHandler(groups, repo, clock)

	_, first := siteTestGet(t, h, siteTestPath, nil)
	siteTestGet(t, h, siteTestPath, nil)
	require.Equal(t, int64(1), repo.calls.Load(), "落稳之前重算也不会更准，直接用缓存")

	// 04:36：过了落稳时间，允许再算一次，把刚结束的那个桶补全
	clock.Set(time.Date(2026, 11, 27, 4, 36, 0, 0, time.UTC))
	_, second := siteTestGet(t, h, siteTestPath, nil)
	require.Equal(t, int64(2), repo.calls.Load())
	require.NotEqual(t, first.UpdatedAt, second.UpdatedAt)

	// 此后同一个半小时内不再计算：不管多少请求、什么语言、什么奇怪的请求头
	clock.Set(time.Date(2026, 11, 27, 4, 59, 59, 0, time.UTC))
	for i := 0; i < 200; i++ {
		siteTestGet(t, h, siteTestPath+"?lang=en-US&nonce="+strings.Repeat("x", i%7), map[string]string{"Accept-Language": "xx-" + strings.Repeat("y", i%5)})
	}
	_, en := siteTestGet(t, h, siteTestPath, map[string]string{"Accept-Language": "en-GB"})
	_, zh := siteTestGet(t, h, siteTestPath, map[string]string{"Accept-Language": "fr-FR"})
	require.Equal(t, "G-en", en.Groups[0].Name)
	require.Equal(t, "G", zh.Groups[0].Name, "不认识的语言归到中文，不产生新的缓存项")
	require.Equal(t, int64(2), repo.calls.Load())
	require.Len(t, h.cache.bodies, 2)

	// 进入下一个半小时窗口：重新计算，时间窗跟着前移
	clock.Set(time.Date(2026, 11, 27, 5, 6, 0, 0, time.UTC))
	siteTestGet(t, h, siteTestPath, nil)
	require.Equal(t, int64(3), repo.calls.Load())
	require.Equal(t, time.Date(2026, 11, 27, 5, 0, 0, 0, time.UTC), repo.lastTo)
}

func TestSiteSnapshotHandler_ConcurrentRequestsShareOneComputation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clock := &siteTestClock{now: siteTestNow}
	repo := &stubSiteStatusRepo{delay: 60 * time.Millisecond}
	h := siteTestHandler([]service.PlazaGroup{{ID: 1, Platform: "openai", Name: "G"}}, repo, clock)

	var wg sync.WaitGroup
	codes := make([]int, 40)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, siteTestPath, nil)
			h.GetStatus(c)
			codes[i] = w.Code
		}(i)
	}
	wg.Wait()
	for _, code := range codes {
		require.Equal(t, http.StatusOK, code)
	}
	require.Equal(t, int64(1), repo.calls.Load())
}

func TestSiteSnapshotHandler_FailureServesPreviousSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clock := &siteTestClock{now: siteTestNow}
	repo := &stubSiteStatusRepo{}
	h := siteTestHandler([]service.PlazaGroup{{ID: 1, Platform: "openai", Name: "G"}}, repo, clock)

	_, good := siteTestGet(t, h, siteTestPath, nil)

	// 下一个窗口里查询失败：继续给上一份，updated_at 仍是当时的时间，官网据此提示数据未更新
	repo.err = errors.New("database unavailable")
	clock.Set(siteTestNow.Add(40 * time.Minute))
	w, stale := siteTestGet(t, h, siteTestPath, nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, good.UpdatedAt, stale.UpdatedAt)

	// 恢复后立刻换成新的
	repo.err = nil
	_, fresh := siteTestGet(t, h, siteTestPath, nil)
	require.NotEqual(t, good.UpdatedAt, fresh.UpdatedAt)

	// 从来没算成功过：如实报错，不编数据，也不让浏览器缓存这次失败
	cold := siteTestHandler([]service.PlazaGroup{{ID: 1, Platform: "openai", Name: "G"}},
		&stubSiteStatusRepo{err: errors.New("database unavailable")}, clock)
	w, _ = siteTestGet(t, cold, siteTestPath, nil)
	require.GreaterOrEqual(t, w.Code, 500)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}

func TestSiteSnapshotHandler_ComputationSurvivesCallerDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clock := &siteTestClock{now: siteTestNow}
	repo := &stubSiteStatusRepo{}
	h := siteTestHandler([]service.PlazaGroup{{ID: 1, Platform: "openai", Name: "G"}}, repo, clock)

	// 触发计算的那个访客已经断开：计算照常完成，后面的人直接用结果
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, siteTestPath, nil).WithContext(ctx)
	h.GetStatus(c)

	require.NoError(t, repo.ctxErr, "计算用的上下文不应随请求取消")
	require.NotNil(t, h.cache)
	siteTestGet(t, h, siteTestPath, nil)
	require.Equal(t, int64(1), repo.calls.Load())
}

func TestSiteSnapshotHandler_DoesNotDependOnChannelMonitors(t *testing.T) {
	// 这个接口自己统计请求日志：不引用渠道监控（v1 主动探测、v2 被动聚合）的任何类型或开关。
	src, err := os.ReadFile("site_snapshot_handler.go")
	require.NoError(t, err)
	for _, forbidden := range []string{"ChannelMonitor", "channel_monitor", "PassiveAggregation", "ActiveProbes"} {
		require.NotContains(t, string(src), forbidden)
	}
}
