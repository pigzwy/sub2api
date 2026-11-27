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

func TestSiteSnapshotHandler_WhitelistSchema48BucketsLocalizationAndPrivacy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fixedNow := time.Date(2026, 11, 27, 4, 31, 2, 0, time.UTC)
	expectedEnd := time.Date(2026, 11, 27, 5, 0, 0, 0, time.UTC)
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
								SuccessRate: 1.0,
								TTFT:        service.ChannelMonitorV2Latency{P50Ms: int64Ptr(1750)},
							},
							Health: service.ChannelMonitorV2Health{Overall: "critical", ErrorRate: "healthy", TTFT: "healthy", Cache: "critical"},
						},
						{
							BucketStart: expectedStart.Add(30 * time.Minute),
							Metrics: service.ChannelMonitorV2Metric{
								SuccessRate: 0.942,
								TTFT:        service.ChannelMonitorV2Latency{P50Ms: int64Ptr(3600)},
							},
							Health: service.ChannelMonitorV2Health{Overall: "healthy", ErrorRate: "healthy", TTFT: "warning"},
						},
						{
							BucketStart: expectedStart.Add(60 * time.Minute),
							Metrics: service.ChannelMonitorV2Metric{
								SuccessRate: 0.72,
								TTFT:        service.ChannelMonitorV2Latency{P50Ms: int64Ptr(12000)},
							},
							Health: service.ChannelMonitorV2Health{Overall: "warning", ErrorRate: "critical", TTFT: "warning"},
						},
					},
				},
				// 即使矩阵里混入了专属分组，也绝不能出现在输出里。
				{
					Platform: "anthropic",
					GroupID:  &gid99,
					Metrics:  service.ChannelMonitorV2Metric{SuccessRate: 1.0},
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
	require.False(t, matrix.lastAdmin)
	require.Equal(t, service.ChannelMonitorV2GroupByPlatformGroup, matrix.lastGroup)
	require.Equal(t, 30*time.Minute, matrix.lastFilter.Bucket)
	require.Equal(t, "24h", matrix.lastFilter.Range)
	require.True(t, matrix.lastFilter.RestrictGroups)
	require.Equal(t, []int64{43, 46}, matrix.lastFilter.AllowedGroupIDs)
	require.Equal(t, expectedStart, matrix.lastFilter.Start)
	require.Equal(t, expectedEnd, matrix.lastFilter.End)

	rawBody := w.Body.String()
	lowerBody := strings.ToLower(rawBody)
	for _, forbidden := range []string{"rpm", "tpm", "request", "token", "cache", "score", "threshold", "user", "account", "exclusive", "secret"} {
		require.NotContains(t, lowerBody, forbidden, "response must not leak %q", forbidden)
	}
	// 不得套 {code, message, data} 信封
	require.NotContains(t, rawBody, `"code":`)
	require.NotContains(t, rawBody, `"data":`)

	var resp siteStatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.OK)
	require.Equal(t, "2026-11-27T04:31:02Z", resp.UpdatedAt)
	require.Equal(t, 30, resp.BucketMinutes)
	require.Len(t, resp.Groups, 2)

	// 分组 43：本地化英文名 + 状态无视 cache/overall + 48 格升序 + 首尾时间精确对齐
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
