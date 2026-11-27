package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

type stubSiteMatrixQuerier struct {
	matrix     *service.ChannelMonitorV2Matrix
	err        error
	lastFilter service.ChannelMonitorV2Filter
	lastGroup  service.ChannelMonitorV2GroupBy
	lastAdmin  bool
	called     bool
}

func (s *stubSiteMatrixQuerier) Matrix(
	_ context.Context,
	filter service.ChannelMonitorV2Filter,
	groupBy service.ChannelMonitorV2GroupBy,
	admin bool,
) (*service.ChannelMonitorV2Matrix, error) {
	s.called = true
	s.lastFilter = filter
	s.lastGroup = groupBy
	s.lastAdmin = admin
	return s.matrix, s.err
}

type stubSiteMonitorSettings struct {
	rt service.ChannelMonitorRuntime
}

func (s *stubSiteMonitorSettings) GetChannelMonitorRuntime(context.Context) service.ChannelMonitorRuntime {
	return s.rt
}

func int64Ptr(v int64) *int64 { return &v }

func TestSiteSnapshotHandler_DisabledMonitorReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name string
		rt   service.ChannelMonitorRuntime
	}{
		{name: "monitor disabled", rt: service.ChannelMonitorRuntime{Enabled: false, Mode: service.ChannelMonitorModeV2}},
		{name: "v1 mode", rt: service.ChannelMonitorRuntime{Enabled: true, Mode: service.ChannelMonitorModeV1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &SiteSnapshotHandler{
				plazaService:     &stubSitePlazaLister{},
				monitorV2Service: &stubSiteMatrixQuerier{},
				settingService:   &stubSiteMonitorSettings{rt: tc.rt},
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/internal/site/status", nil)
			h.GetStatus(c)
			require.Equal(t, http.StatusNotFound, w.Code)
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		})
	}

	t.Run("matrix returns disabled error", func(t *testing.T) {
		h := &SiteSnapshotHandler{
			plazaService: &stubSitePlazaLister{
				groups: []service.PlazaGroup{{ID: 1, Name: "Public", Platform: "openai"}},
			},
			monitorV2Service: &stubSiteMatrixQuerier{err: service.ErrChannelMonitorDisabled},
			settingService:   &stubSiteMonitorSettings{rt: service.ChannelMonitorRuntime{Enabled: true, Mode: service.ChannelMonitorModeV2}},
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/internal/site/status", nil)
		h.GetStatus(c)
		require.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestSiteSnapshotHandler_WhitelistSchemaClosed48BucketsLocalizationAndPrivacy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 04:35:02Z → 时间窗结束于刚过去的半点 04:30:00Z（全部 48 桶均已结束，不含进行中的 04:30-05:00）
	fixedNow := time.Date(2026, 11, 27, 4, 35, 2, 0, time.UTC)
	expectedEnd := time.Date(2026, 11, 27, 4, 30, 0, 0, time.UTC)
	expectedStart := expectedEnd.Add(-24 * time.Hour)

	gid43 := int64(43)
	gid46 := int64(46)
	gid99 := int64(99)

	plaza := &stubSitePlazaLister{
		groups: []service.PlazaGroup{
			{
				ID:       gid43,
				Platform: "anthropic",
				Name:     "企业 CC-MAX | 限客户端",
				I18n: service.GroupI18n{
					"en-US": {Name: "Enterprise CC-MAX | Client Only"},
				},
			},
			{
				ID:       gid46,
				Platform: "openai",
				Name:     "企业 Codex | 福利",
				I18n: service.GroupI18n{
					"en-US": {Name: "Enterprise Codex | Bonus"},
				},
			},
			{
				ID:          gid99,
				Platform:    "anthropic",
				Name:        "VIP Exclusive Secret",
				IsExclusive: true,
			},
		},
	}

	matrix := &stubSiteMatrixQuerier{
		matrix: &service.ChannelMonitorV2Matrix{
			GroupBy: service.ChannelMonitorV2GroupByPlatformGroup,
			Items: []service.ChannelMonitorV2MatrixRow{
				{
					Platform:  "anthropic",
					GroupID:   &gid43,
					GroupName: "企业 CC-MAX | 限客户端",
					Metrics: service.ChannelMonitorV2Metric{
						RequestCount: 5000,
						TokenCount:   999999,
						RPM:          120.5,
						TPM:          45000.2,
						CacheRate:    0.01,
						SuccessRate:  0.9984,
						TTFT:         service.ChannelMonitorV2Latency{SampleCount: 500, P50Ms: int64Ptr(1800)},
					},
					// Overall 故意设为 critical 且 Cache=critical，验证状态仅由 ErrorRate 与 TTFT 决定。
					Health: service.ChannelMonitorV2Health{
						Overall:   "critical",
						ErrorRate: "healthy",
						TTFT:      "healthy",
						Cache:     "critical",
					},
					Buckets: []service.ChannelMonitorV2TrendPoint{
						{
							BucketStart: expectedStart,
							Metrics: service.ChannelMonitorV2Metric{
								RequestCount: 100,
								SuccessRate:  1.0,
								TTFT:         service.ChannelMonitorV2Latency{SampleCount: 80, P50Ms: int64Ptr(1750)},
							},
							Health: service.ChannelMonitorV2Health{Overall: "critical", ErrorRate: "healthy", TTFT: "healthy", Cache: "critical"},
						},
						{
							BucketStart: expectedStart.Add(30 * time.Minute),
							Metrics: service.ChannelMonitorV2Metric{
								RequestCount: 100,
								SuccessRate:  0.942,
								TTFT:         service.ChannelMonitorV2Latency{SampleCount: 90, P50Ms: int64Ptr(3600)},
							},
							Health: service.ChannelMonitorV2Health{Overall: "healthy", ErrorRate: "healthy", TTFT: "warning"},
						},
						{
							BucketStart: expectedStart.Add(60 * time.Minute),
							Metrics: service.ChannelMonitorV2Metric{
								RequestCount: 100,
								SuccessRate:  0.72,
								TTFT:         service.ChannelMonitorV2Latency{SampleCount: 90, P50Ms: int64Ptr(12000)},
							},
							Health: service.ChannelMonitorV2Health{Overall: "warning", ErrorRate: "critical", TTFT: "warning"},
						},
						// 最新一个已结束桶（第 48 格，索引 47）正常，决定当前分组状态为 normal
						{
							BucketStart: expectedEnd.Add(-30 * time.Minute),
							Metrics: service.ChannelMonitorV2Metric{
								RequestCount: 120,
								SuccessRate:  1.0,
								TTFT:         service.ChannelMonitorV2Latency{SampleCount: 100, P50Ms: int64Ptr(1600)},
							},
							Health: service.ChannelMonitorV2Health{Overall: "critical", ErrorRate: "healthy", TTFT: "healthy", Cache: "critical"},
						},
					},
				},
				// 即使矩阵里混入了专属分组，也绝不能出现在输出里。
				{
					Platform: "anthropic",
					GroupID:  &gid99,
					Metrics:  service.ChannelMonitorV2Metric{RequestCount: 100, SuccessRate: 1.0},
					Health:   service.ChannelMonitorV2Health{ErrorRate: "healthy", TTFT: "healthy"},
				},
			},
		},
	}

	h := &SiteSnapshotHandler{
		plazaService:     plaza,
		monitorV2Service: matrix,
		settingService:   &stubSiteMonitorSettings{rt: service.ChannelMonitorRuntime{Enabled: true, Mode: service.ChannelMonitorModeV2}},
		now:              func() time.Time { return fixedNow },
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/internal/site/status", nil)
	c.Request.Header.Set("Accept-Language", "en-US")

	h.GetStatus(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.True(t, matrix.called)
	// 内部拉取未脱敏矩阵用于多平台加权合并，响应仍严格走白名单。
	require.True(t, matrix.lastAdmin)
	require.Equal(t, service.ChannelMonitorV2GroupByPlatformGroup, matrix.lastGroup)
	require.Equal(t, 30*time.Minute, matrix.lastFilter.Bucket)
	require.Equal(t, "24h", matrix.lastFilter.Range)
	require.True(t, matrix.lastFilter.RestrictGroups)
	require.Equal(t, []int64{43, 46}, matrix.lastFilter.AllowedGroupIDs)
	require.Equal(t, expectedStart, matrix.lastFilter.Start)
	require.Equal(t, expectedEnd, matrix.lastFilter.End)

	rawBody := w.Body.String()
	lowerBody := strings.ToLower(rawBody)
	for _, forbidden := range []string{"rpm", "tpm", "request", "token", "cache", "score", "threshold", "user", "account", "exclusive", "secret", "sample"} {
		require.NotContains(t, lowerBody, forbidden, "response must not leak %q", forbidden)
	}
	// 不得套 {code, message, data} 信封
	require.NotContains(t, rawBody, `"code":`)
	require.NotContains(t, rawBody, `"data":`)

	var resp siteStatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.OK)
	require.Equal(t, "2026-11-27T04:35:02Z", resp.UpdatedAt)
	require.Equal(t, 30, resp.BucketMinutes)
	require.Len(t, resp.Groups, 2)

	// 分组 43：本地化英文名 + 状态无视 cache/overall + 48 个已结束桶升序 + 首尾时间精确对齐
	g0 := resp.Groups[0]
	require.Equal(t, int64(43), g0.ID)
	require.Equal(t, "anthropic", g0.Platform)
	require.Equal(t, "Enterprise CC-MAX | Client Only", g0.Name)
	require.Equal(t, "normal", g0.Status)
	require.NotNil(t, g0.Uptime24h)
	require.InDelta(t, 99.8, *g0.Uptime24h, 0.001)
	require.NotNil(t, g0.TTFTMs)
	require.Equal(t, int64(1800), *g0.TTFTMs)
	require.Len(t, g0.Buckets, 48)
	require.Equal(t, expectedStart.Format(time.RFC3339), g0.Buckets[0].Start)
	require.Equal(t, expectedEnd.Format(time.RFC3339), g0.Buckets[47].End)
	for i := 1; i < len(g0.Buckets); i++ {
		require.Equal(t, g0.Buckets[i-1].End, g0.Buckets[i].Start, "buckets must be contiguous ascending")
	}
	require.Equal(t, "normal", g0.Buckets[0].Status)
	require.InDelta(t, 100.0, *g0.Buckets[0].Uptime, 0.001)
	require.Equal(t, int64(1750), *g0.Buckets[0].TTFTMs)
	require.Equal(t, "degraded", g0.Buckets[1].Status)
	require.Equal(t, "outage", g0.Buckets[2].Status)
	require.Equal(t, "no_data", g0.Buckets[3].Status)
	require.Nil(t, g0.Buckets[3].Uptime)
	require.Nil(t, g0.Buckets[3].TTFTMs)
	require.Equal(t, "normal", g0.Buckets[47].Status)

	// 分组 46：矩阵中无流量 → 仍输出且 48 格全为 no_data + uptime/ttft 为 null
	g1 := resp.Groups[1]
	require.Equal(t, int64(46), g1.ID)
	require.Equal(t, "Enterprise Codex | Bonus", g1.Name)
	require.Equal(t, "no_data", g1.Status)
	require.Nil(t, g1.Uptime24h)
	require.Nil(t, g1.TTFTMs)
	require.Len(t, g1.Buckets, 48)
	for _, b := range g1.Buckets {
		require.Equal(t, "no_data", b.Status)
		require.Nil(t, b.Uptime)
		require.Nil(t, b.TTFTMs)
	}
}

func TestSiteSnapshotHandler_CompositeGroupMergesMultiPlatformRows(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fixedNow := time.Date(2026, 11, 27, 4, 35, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 11, 27, 4, 30, 0, 0, time.UTC)
	lastBucket := windowEnd.Add(-30 * time.Minute)
	prevBucket := windowEnd.Add(-60 * time.Minute)

	gid71 := int64(71)
	plaza := &stubSitePlazaLister{
		groups: []service.PlazaGroup{
			{
				ID:       gid71,
				Platform: "composite",
				Name:     "图像视频 | Image | Video | Voice",
			},
		},
	}

	// 同一个组合分组 71 在矩阵中拆成 openai 和 gemini 两行：
	// - 24h 总计：openai(300 req, 100%, ttft 1200ms/280 samples, healthy) + gemini(100 req, 80%, ttft 4200ms/90 samples, warning)
	//   → 合并后 uptime_24h = (300*1.0 + 100*0.8)/400 = 95.0%，ttft_ms = 1200（样本最多者）
	// - prevBucket：仅 openai 有数据(normal)，gemini 无该桶 → 保留 openai 数据不被覆盖
	// - lastBucket：openai(normal, 80 req, 100%, ttft 1100ms/60 samples) + gemini(outage, 20 req, 50%, ttft 6000ms/15 samples)
	//   → 合并状态取最差 outage，加权可用率 = (80*1.0 + 20*0.5)/100 = 90.0%，TTFT 取样本最多的 1100ms
	matrix := &stubSiteMatrixQuerier{
		matrix: &service.ChannelMonitorV2Matrix{
			GroupBy: service.ChannelMonitorV2GroupByPlatformGroup,
			Items: []service.ChannelMonitorV2MatrixRow{
				{
					Platform: "openai",
					GroupID:  &gid71,
					Metrics: service.ChannelMonitorV2Metric{
						RequestCount: 300,
						SuccessRate:  1.0,
						TTFT:         service.ChannelMonitorV2Latency{SampleCount: 280, P50Ms: int64Ptr(1200)},
					},
					Health: service.ChannelMonitorV2Health{ErrorRate: "healthy", TTFT: "healthy"},
					Buckets: []service.ChannelMonitorV2TrendPoint{
						{
							BucketStart: prevBucket,
							Metrics: service.ChannelMonitorV2Metric{
								RequestCount: 50,
								SuccessRate:  0.98,
								TTFT:         service.ChannelMonitorV2Latency{SampleCount: 45, P50Ms: int64Ptr(1300)},
							},
							Health: service.ChannelMonitorV2Health{ErrorRate: "healthy", TTFT: "healthy"},
						},
						{
							BucketStart: lastBucket,
							Metrics: service.ChannelMonitorV2Metric{
								RequestCount: 80,
								SuccessRate:  1.0,
								TTFT:         service.ChannelMonitorV2Latency{SampleCount: 60, P50Ms: int64Ptr(1100)},
							},
							Health: service.ChannelMonitorV2Health{ErrorRate: "healthy", TTFT: "healthy"},
						},
					},
				},
				{
					Platform: "gemini",
					GroupID:  &gid71,
					Metrics: service.ChannelMonitorV2Metric{
						RequestCount: 100,
						SuccessRate:  0.80,
						TTFT:         service.ChannelMonitorV2Latency{SampleCount: 90, P50Ms: int64Ptr(4200)},
					},
					Health: service.ChannelMonitorV2Health{ErrorRate: "warning", TTFT: "healthy"},
					Buckets: []service.ChannelMonitorV2TrendPoint{
						{
							BucketStart: lastBucket,
							Metrics: service.ChannelMonitorV2Metric{
								RequestCount: 20,
								SuccessRate:  0.50,
								TTFT:         service.ChannelMonitorV2Latency{SampleCount: 15, P50Ms: int64Ptr(6000)},
							},
							Health: service.ChannelMonitorV2Health{ErrorRate: "critical", TTFT: "warning"},
						},
					},
				},
			},
		},
	}

	h := &SiteSnapshotHandler{
		plazaService:     plaza,
		monitorV2Service: matrix,
		settingService:   &stubSiteMonitorSettings{rt: service.ChannelMonitorRuntime{Enabled: true, Mode: service.ChannelMonitorModeV2}},
		now:              func() time.Time { return fixedNow },
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/internal/site/status", nil)
	h.GetStatus(c)

	require.Equal(t, http.StatusOK, w.Code)
	lowerBody := strings.ToLower(w.Body.String())
	for _, forbidden := range []string{"request", "sample", "rpm", "tpm", "token", "cache"} {
		require.NotContains(t, lowerBody, forbidden)
	}

	var resp siteStatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Groups, 1)

	g := resp.Groups[0]
	require.Equal(t, gid71, g.ID)
	require.Equal(t, "composite", g.Platform)
	// 当前状态反映最新桶的合并结果 outage
	require.Equal(t, "outage", g.Status)
	// 24h 可用率按请求数加权 = 95.0%，TTFT 取样本最多的 openai = 1200ms
	require.NotNil(t, g.Uptime24h)
	require.InDelta(t, 95.0, *g.Uptime24h, 0.001)
	require.NotNil(t, g.TTFTMs)
	require.Equal(t, int64(1200), *g.TTFTMs)

	// 倒数第 2 桶（索引 46）保留 openai 数据未被空 gemini 行覆盖
	b46 := g.Buckets[46]
	require.Equal(t, "normal", b46.Status)
	require.NotNil(t, b46.Uptime)
	require.InDelta(t, 98.0, *b46.Uptime, 0.001)
	require.NotNil(t, b46.TTFTMs)
	require.Equal(t, int64(1300), *b46.TTFTMs)

	// 最新一桶（索引 47）合并结果：状态 outage、加权可用率 90.0%、TTFT 1100ms
	b47 := g.Buckets[47]
	require.Equal(t, "outage", b47.Status)
	require.NotNil(t, b47.Uptime)
	require.InDelta(t, 90.0, *b47.Uptime, 0.001)
	require.NotNil(t, b47.TTFTMs)
	require.Equal(t, int64(1100), *b47.TTFTMs)
}

func TestSiteSnapshotHandler_CurrentStatusUsesRecentClosedBucketsNot24hAverage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fixedNow := time.Date(2026, 11, 27, 4, 35, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 11, 27, 4, 30, 0, 0, time.UTC)
	windowStart := windowEnd.Add(-24 * time.Hour)

	gidFreshOutage := int64(101)
	gidRecovered := int64(102)
	gidRecentSkipNoData := int64(103)
	gidIdleNow := int64(104)

	plaza := &stubSitePlazaLister{
		groups: []service.PlazaGroup{
			{ID: gidFreshOutage, Platform: "anthropic", Name: "Fresh Outage"},
			{ID: gidRecovered, Platform: "openai", Name: "Recovered"},
			{ID: gidRecentSkipNoData, Platform: "grok", Name: "Recent Skip NoData"},
			{ID: gidIdleNow, Platform: "deepseek", Name: "Idle Now"},
		},
	}

	matrix := &stubSiteMatrixQuerier{
		matrix: &service.ChannelMonitorV2Matrix{
			GroupBy: service.ChannelMonitorV2GroupByPlatformGroup,
			Items: []service.ChannelMonitorV2MatrixRow{
				// 1. 前 23h 很健康导致 24h 整行 healthy，但最新一桶刚发生 outage → 当前状态必须是 outage，24h 可用率保留 99.2%
				{
					Platform: "anthropic",
					GroupID:  &gidFreshOutage,
					Metrics: service.ChannelMonitorV2Metric{
						RequestCount: 5000,
						SuccessRate:  0.992,
						TTFT:         service.ChannelMonitorV2Latency{SampleCount: 4000, P50Ms: int64Ptr(1500)},
					},
					Health: service.ChannelMonitorV2Health{ErrorRate: "healthy", TTFT: "healthy"},
					Buckets: []service.ChannelMonitorV2TrendPoint{
						{
							BucketStart: windowEnd.Add(-30 * time.Minute),
							Metrics:     service.ChannelMonitorV2Metric{RequestCount: 50, SuccessRate: 0.4, TTFT: service.ChannelMonitorV2Latency{SampleCount: 50, P50Ms: int64Ptr(9000)}},
							Health:      service.ChannelMonitorV2Health{ErrorRate: "critical", TTFT: "critical"},
						},
					},
				},
				// 2. 12 小时前波动导致 24h 整行 warning，但最近已恢复 normal → 当前状态必须是 normal
				{
					Platform: "openai",
					GroupID:  &gidRecovered,
					Metrics: service.ChannelMonitorV2Metric{
						RequestCount: 2000,
						SuccessRate:  0.965,
						TTFT:         service.ChannelMonitorV2Latency{SampleCount: 1800, P50Ms: int64Ptr(1700)},
					},
					Health: service.ChannelMonitorV2Health{ErrorRate: "warning", TTFT: "healthy"},
					Buckets: []service.ChannelMonitorV2TrendPoint{
						{
							BucketStart: windowStart.Add(12 * time.Hour),
							Metrics:     service.ChannelMonitorV2Metric{RequestCount: 200, SuccessRate: 0.70},
							Health:      service.ChannelMonitorV2Health{ErrorRate: "critical", TTFT: "healthy"},
						},
						{
							BucketStart: windowEnd.Add(-30 * time.Minute),
							Metrics:     service.ChannelMonitorV2Metric{RequestCount: 100, SuccessRate: 1.0, TTFT: service.ChannelMonitorV2Latency{SampleCount: 90, P50Ms: int64Ptr(1100)}},
							Health:      service.ChannelMonitorV2Health{ErrorRate: "healthy", TTFT: "healthy"},
						},
					},
				},
				// 3. 最新桶无流量，倒数第 3 桶（1.5h 内）为 degraded → 取最近 3 桶内最新有数据的 degraded
				{
					Platform: "grok",
					GroupID:  &gidRecentSkipNoData,
					Metrics: service.ChannelMonitorV2Metric{
						RequestCount: 300,
						SuccessRate:  0.98,
						TTFT:         service.ChannelMonitorV2Latency{SampleCount: 250, P50Ms: int64Ptr(2100)},
					},
					Health: service.ChannelMonitorV2Health{ErrorRate: "healthy", TTFT: "healthy"},
					Buckets: []service.ChannelMonitorV2TrendPoint{
						{
							BucketStart: windowEnd.Add(-90 * time.Minute), // index 45
							Metrics:     service.ChannelMonitorV2Metric{RequestCount: 40, SuccessRate: 0.92},
							Health:      service.ChannelMonitorV2Health{ErrorRate: "warning", TTFT: "healthy"},
						},
					},
				},
				// 4. 仅 2 小时前（索引 44，超出最近 3 桶）有流量，最近 3 桶全部无流量 → 当前状态为 no_data，但 24h 可用率保留 99.5%
				{
					Platform: "deepseek",
					GroupID:  &gidIdleNow,
					Metrics: service.ChannelMonitorV2Metric{
						RequestCount: 400,
						SuccessRate:  0.995,
						TTFT:         service.ChannelMonitorV2Latency{SampleCount: 380, P50Ms: int64Ptr(950)},
					},
					Health: service.ChannelMonitorV2Health{ErrorRate: "healthy", TTFT: "healthy"},
					Buckets: []service.ChannelMonitorV2TrendPoint{
						{
							BucketStart: windowEnd.Add(-120 * time.Minute), // index 44
							Metrics:     service.ChannelMonitorV2Metric{RequestCount: 100, SuccessRate: 1.0},
							Health:      service.ChannelMonitorV2Health{ErrorRate: "healthy", TTFT: "healthy"},
						},
					},
				},
			},
		},
	}

	h := &SiteSnapshotHandler{
		plazaService:     plaza,
		monitorV2Service: matrix,
		settingService:   &stubSiteMonitorSettings{rt: service.ChannelMonitorRuntime{Enabled: true, Mode: service.ChannelMonitorModeV2}},
		now:              func() time.Time { return fixedNow },
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/internal/site/status", nil)
	h.GetStatus(c)
	require.Equal(t, http.StatusOK, w.Code)

	var resp siteStatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Groups, 4)

	// 1. Fresh outage 不被 24h 稀释，且 24h 可用率保持 99.2%
	require.Equal(t, "outage", resp.Groups[0].Status)
	require.NotNil(t, resp.Groups[0].Uptime24h)
	require.InDelta(t, 99.2, *resp.Groups[0].Uptime24h, 0.001)

	// 2. 历史波动恢复后当前状态为 normal，24h 可用率保持 96.5%
	require.Equal(t, "normal", resp.Groups[1].Status)
	require.NotNil(t, resp.Groups[1].Uptime24h)
	require.InDelta(t, 96.5, *resp.Groups[1].Uptime24h, 0.001)

	// 3. 最近 3 桶内最新有数据项为 degraded
	require.Equal(t, "degraded", resp.Groups[2].Status)

	// 4. 最近 3 桶均无数据 → 当前状态 no_data，24h 可用率与 TTFT 不变
	require.Equal(t, "no_data", resp.Groups[3].Status)
	require.NotNil(t, resp.Groups[3].Uptime24h)
	require.InDelta(t, 99.5, *resp.Groups[3].Uptime24h, 0.001)
	require.NotNil(t, resp.Groups[3].TTFTMs)
	require.Equal(t, int64(950), *resp.Groups[3].TTFTMs)
}

func TestMapSiteHealthStatus_AllFourStates(t *testing.T) {
	require.Equal(t, "normal", mapSiteHealthStatus("healthy", "healthy"))
	require.Equal(t, "normal", mapSiteHealthStatus("healthy", "unknown"))
	require.Equal(t, "degraded", mapSiteHealthStatus("healthy", "warning"))
	require.Equal(t, "degraded", mapSiteHealthStatus("warning", "unknown"))
	require.Equal(t, "outage", mapSiteHealthStatus("warning", "critical"))
	require.Equal(t, "outage", mapSiteHealthStatus("critical", "healthy"))
	require.Equal(t, "no_data", mapSiteHealthStatus("unknown", "unknown"))
	require.Equal(t, "no_data", mapSiteHealthStatus("", ""))
}
