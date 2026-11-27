package handler

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	siteSnapshotBucketDuration      = 30 * time.Minute
	siteSnapshotBucketMinutes       = 30
	siteSnapshotBucketCount         = 48
	siteSnapshotCurrentLookbackBins = 3 // 取最近 3 个已结束半小时桶中最新有数据的状态作为分组当前状态
)

type siteSnapshotPlazaLister interface {
	ListGroups(ctx context.Context) ([]service.PlazaGroup, error)
}

type siteSnapshotMatrixQuerier interface {
	Matrix(ctx context.Context, filter service.ChannelMonitorV2Filter, groupBy service.ChannelMonitorV2GroupBy, admin bool) (*service.ChannelMonitorV2Matrix, error)
}

type siteSnapshotMonitorSettings interface {
	GetChannelMonitorRuntime(ctx context.Context) service.ChannelMonitorRuntime
}

// SiteSnapshotHandler 为官网本机定时快照提供脱敏后的公开分组健康状态。
// 仅输出严格白名单字段，绝不暴露请求数、令牌数、RPM/TPM、缓存率、阈值、用户或账号信息。
type SiteSnapshotHandler struct {
	plazaService     siteSnapshotPlazaLister
	monitorV2Service siteSnapshotMatrixQuerier
	settingService   siteSnapshotMonitorSettings
	now              func() time.Time
}

// NewSiteSnapshotHandler 创建官网状态快照 handler。
func NewSiteSnapshotHandler(
	plazaService *service.ModelPlazaService,
	monitorV2Service *service.ChannelMonitorV2Service,
	settingService *service.SettingService,
) *SiteSnapshotHandler {
	return &SiteSnapshotHandler{
		plazaService:     plazaService,
		monitorV2Service: monitorV2Service,
		settingService:   settingService,
		now:              time.Now,
	}
}

type siteStatusBucket struct {
	Start  string   `json:"start"`
	End    string   `json:"end"`
	Status string   `json:"status"`
	Uptime *float64 `json:"uptime"`
	TTFTMs *int64   `json:"ttft_ms"`
}

type siteStatusGroup struct {
	ID        int64              `json:"id"`
	Platform  string             `json:"platform"`
	Name      string             `json:"name"`
	Status    string             `json:"status"`
	Uptime24h *float64           `json:"uptime_24h"`
	TTFTMs    *int64             `json:"ttft_ms"`
	Buckets   []siteStatusBucket `json:"buckets"`
}

type siteStatusResponse struct {
	OK            bool              `json:"ok"`
	UpdatedAt     string            `json:"updated_at"`
	BucketMinutes int               `json:"bucket_minutes"`
	Groups        []siteStatusGroup `json:"groups"`
}

type siteMetricSample struct {
	metrics service.ChannelMonitorV2Metric
	health  service.ChannelMonitorV2Health
}

// GetStatus 返回最近 24 小时（48 个已结束的 30 分钟桶）的公开分组脱敏健康状态。
// GET /api/v1/internal/site/status
func (h *SiteSnapshotHandler) GetStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Writer.Header().Add("Vary", "Accept-Language")

	if h == nil || h.plazaService == nil || h.monitorV2Service == nil || h.settingService == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	ctx := c.Request.Context()
	if !h.settingService.GetChannelMonitorRuntime(ctx).PassiveAggregationAllowed() {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	rawGroups, err := h.plazaService.ListGroups(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	visible := filterPlazaVisibleGroups(rawGroups, nil, false)

	nowFn := h.now
	if nowFn == nil {
		nowFn = time.Now
	}
	nowUTC := nowFn().UTC()
	// 时间窗结束于刚过去的半点，确保 48 个桶全部是已结束的完整半小时窗口。
	windowEnd := nowUTC.Truncate(siteSnapshotBucketDuration)
	windowStart := windowEnd.Add(-24 * time.Hour)

	// 组合分组在监控矩阵中会按实际账号平台拆成多行，这里按 GroupID 收集所有行再合并。
	rowsByGroup := make(map[int64][]*service.ChannelMonitorV2MatrixRow, len(visible))
	if len(visible) > 0 {
		allowedIDs := make([]int64, 0, len(visible))
		for i := range visible {
			allowedIDs = append(allowedIDs, visible[i].ID)
		}
		filter := service.ChannelMonitorV2Filter{
			Range:           "24h",
			Start:           windowStart,
			End:             windowEnd,
			Bucket:          siteSnapshotBucketDuration,
			RestrictGroups:  true,
			AllowedGroupIDs: allowedIDs,
		}
		// 传 admin=true 仅用于在接口内部读取 RequestCount 与 TTFT.SampleCount 做多平台加权合并，
		// 最终响应严格走白名单结构体，不暴露任何绝对计数或吞吐量字段。
		matrix, err := h.monitorV2Service.Matrix(ctx, filter, service.ChannelMonitorV2GroupByPlatformGroup, true)
		if err != nil {
			if errors.Is(err, service.ErrChannelMonitorDisabled) {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}
			response.ErrorFrom(c, err)
			return
		}
		if matrix != nil {
			for i := range matrix.Items {
				row := &matrix.Items[i]
				if row.GroupID != nil && *row.GroupID > 0 {
					rowsByGroup[*row.GroupID] = append(rowsByGroup[*row.GroupID], row)
				}
			}
		}
	}

	acceptLang := c.GetHeader("Accept-Language")
	outGroups := make([]siteStatusGroup, 0, len(visible))
	for i := range visible {
		g := &visible[i]
		name, _ := g.I18n.Localize(acceptLang, g.Name, g.Description)
		rows := rowsByGroup[g.ID]
		outGroups = append(outGroups, buildSiteStatusGroup(g.ID, g.Platform, name, rows, windowStart))
	}

	c.JSON(http.StatusOK, siteStatusResponse{
		OK:            true,
		UpdatedAt:     nowUTC.Format(time.RFC3339),
		BucketMinutes: siteSnapshotBucketMinutes,
		Groups:        outGroups,
	})
}

func buildSiteStatusGroup(
	id int64,
	platform string,
	name string,
	rows []*service.ChannelMonitorV2MatrixRow,
	windowStart time.Time,
) siteStatusGroup {
	bucketSamples := make(map[int64][]siteMetricSample)
	rowSamples := make([]siteMetricSample, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		rowSamples = append(rowSamples, siteMetricSample{
			metrics: row.Metrics,
			health:  row.Health,
		})
		for i := range row.Buckets {
			pt := &row.Buckets[i]
			key := pt.BucketStart.UTC().Truncate(siteSnapshotBucketDuration).Unix()
			bucketSamples[key] = append(bucketSamples[key], siteMetricSample{
				metrics: pt.Metrics,
				health:  pt.Health,
			})
		}
	}

	buckets := make([]siteStatusBucket, 0, siteSnapshotBucketCount)
	for i := 0; i < siteSnapshotBucketCount; i++ {
		bStart := windowStart.Add(time.Duration(i) * siteSnapshotBucketDuration)
		bEnd := bStart.Add(siteSnapshotBucketDuration)
		st, up, ttft := mergeSiteSamples(bucketSamples[bStart.Unix()])
		buckets = append(buckets, siteStatusBucket{
			Start:  bStart.Format(time.RFC3339),
			End:    bEnd.Format(time.RFC3339),
			Status: st,
			Uptime: up,
			TTFTMs: ttft,
		})
	}

	// 24 小时可用率与首字延迟保持 24h 汇总（多平台行合并）；
	// 分组当前状态则取最近 3 个已结束桶中最新一个有数据的状态，避免被前 23 小时稀释或卡在历史波动。
	_, uptime24h, ttftMs := mergeSiteSamples(rowSamples)
	currentStatus := resolveCurrentSiteStatus(buckets, siteSnapshotCurrentLookbackBins)

	return siteStatusGroup{
		ID:        id,
		Platform:  platform,
		Name:      name,
		Status:    currentStatus,
		Uptime24h: uptime24h,
		TTFTMs:    ttftMs,
		Buckets:   buckets,
	}
}

// mergeSiteSamples 合并同一分组的多平台监控样本：
// - 状态取最差项（outage > degraded > normal > no_data），仅看 ErrorRate 与 TTFT，绝不混入缓存命中率；
// - 可用率按 RequestCount 加权（无请求数时退回有数据样本的均值）；
// - 首字延迟 P50 取 TTFT 样本数最多那行的值（同样本数时按请求数、较高延迟稳定决胜）；
// - 合并后状态为 no_data 时，uptime 与 ttft_ms 一律输出 nil。
func mergeSiteSamples(samples []siteMetricSample) (string, *float64, *int64) {
	if len(samples) == 0 {
		return "no_data", nil, nil
	}

	worstRank := 0
	var weightedSuccess float64
	var totalRequests int64
	var fallbackRateSum float64
	var fallbackRateCount int

	var bestTTFT *int64
	var bestTTFTSamples int64 = -1
	var bestTTFTReqs int64 = -1

	for _, s := range samples {
		st := mapSiteHealthStatus(s.health.ErrorRate, s.health.TTFT)
		if r := siteStatusRank(st); r > worstRank {
			worstRank = r
		}
		if s.metrics.RequestCount > 0 && !math.IsNaN(s.metrics.SuccessRate) && !math.IsInf(s.metrics.SuccessRate, 0) {
			weightedSuccess += s.metrics.SuccessRate * float64(s.metrics.RequestCount)
			totalRequests += s.metrics.RequestCount
		}
		if st != "no_data" && !math.IsNaN(s.metrics.SuccessRate) && !math.IsInf(s.metrics.SuccessRate, 0) {
			fallbackRateSum += s.metrics.SuccessRate
			fallbackRateCount++
		}
		if s.metrics.TTFT.P50Ms != nil && *s.metrics.TTFT.P50Ms >= 0 {
			sc := s.metrics.TTFT.SampleCount
			rc := s.metrics.RequestCount
			val := *s.metrics.TTFT.P50Ms
			if bestTTFT == nil ||
				sc > bestTTFTSamples ||
				(sc == bestTTFTSamples && rc > bestTTFTReqs) ||
				(sc == bestTTFTSamples && rc == bestTTFTReqs && val > *bestTTFT) {
				bestTTFT = cloneSiteTTFT(s.metrics.TTFT.P50Ms)
				bestTTFTSamples = sc
				bestTTFTReqs = rc
			}
		}
	}

	status := siteStatusFromRank(worstRank)
	if status == "no_data" {
		return "no_data", nil, nil
	}

	var uptime *float64
	if totalRequests > 0 {
		uptime = roundSitePercent(weightedSuccess / float64(totalRequests))
	} else if fallbackRateCount > 0 {
		uptime = roundSitePercent(fallbackRateSum / float64(fallbackRateCount))
	}
	return status, uptime, bestTTFT
}

// resolveCurrentSiteStatus 取最近 lookback 个已结束桶中最新一个非 no_data 状态；全无数据返回 no_data。
func resolveCurrentSiteStatus(buckets []siteStatusBucket, lookback int) string {
	if len(buckets) == 0 || lookback <= 0 {
		return "no_data"
	}
	start := len(buckets) - lookback
	if start < 0 {
		start = 0
	}
	for i := len(buckets) - 1; i >= start; i-- {
		if buckets[i].Status != "" && buckets[i].Status != "no_data" {
			return buckets[i].Status
		}
	}
	return "no_data"
}

// mapSiteHealthStatus 仅由错误率与首字延迟两档决定状态，取较差项，绝不混入缓存命中率。
func mapSiteHealthStatus(errorRateBand, ttftBand string) string {
	worst := siteHealthRank(errorRateBand)
	if r := siteHealthRank(ttftBand); r > worst {
		worst = r
	}
	return siteStatusFromRank(worst)
}

func siteHealthRank(band string) int {
	switch strings.ToLower(strings.TrimSpace(band)) {
	case "critical":
		return 3
	case "warning":
		return 2
	case "healthy":
		return 1
	default:
		return 0
	}
}

func siteStatusRank(status string) int {
	switch status {
	case "outage":
		return 3
	case "degraded":
		return 2
	case "normal":
		return 1
	default:
		return 0
	}
}

func siteStatusFromRank(rank int) string {
	switch rank {
	case 3:
		return "outage"
	case 2:
		return "degraded"
	case 1:
		return "normal"
	default:
		return "no_data"
	}
}

func roundSitePercent(rate float64) *float64 {
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return nil
	}
	pct := math.Round(rate*1000) / 10
	if pct < 0 {
		pct = 0
	} else if pct > 100 {
		pct = 100
	}
	return &pct
}

func cloneSiteTTFT(v *int64) *int64 {
	if v == nil || *v < 0 {
		return nil
	}
	cp := *v
	return &cp
}
