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
	siteSnapshotBucketDuration = 30 * time.Minute
	siteSnapshotBucketMinutes  = 30
	siteSnapshotBucketCount    = 48
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

// GetStatus 返回最近 24 小时（每 30 分钟一桶，共 48 桶）的公开分组脱敏健康状态。
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
	windowEnd := nowUTC.Truncate(siteSnapshotBucketDuration).Add(siteSnapshotBucketDuration)
	windowStart := windowEnd.Add(-24 * time.Hour)

	matrixByGroup := make(map[int64]*service.ChannelMonitorV2MatrixRow, len(visible))
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
		matrix, err := h.monitorV2Service.Matrix(ctx, filter, service.ChannelMonitorV2GroupByPlatformGroup, false)
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
					matrixByGroup[*row.GroupID] = row
				}
			}
		}
	}

	acceptLang := c.GetHeader("Accept-Language")
	outGroups := make([]siteStatusGroup, 0, len(visible))
	for i := range visible {
		g := &visible[i]
		name, _ := g.I18n.Localize(acceptLang, g.Name, g.Description)
		row := matrixByGroup[g.ID]
		outGroups = append(outGroups, buildSiteStatusGroup(g.ID, g.Platform, name, row, windowStart))
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
	row *service.ChannelMonitorV2MatrixRow,
	windowStart time.Time,
) siteStatusGroup {
	bucketMap := make(map[int64]*service.ChannelMonitorV2TrendPoint)
	if row != nil {
		for i := range row.Buckets {
			pt := &row.Buckets[i]
			key := pt.BucketStart.UTC().Truncate(siteSnapshotBucketDuration).Unix()
			bucketMap[key] = pt
		}
	}

	buckets := make([]siteStatusBucket, 0, siteSnapshotBucketCount)
	for i := 0; i < siteSnapshotBucketCount; i++ {
		bStart := windowStart.Add(time.Duration(i) * siteSnapshotBucketDuration)
		bEnd := bStart.Add(siteSnapshotBucketDuration)
		item := siteStatusBucket{
			Start:  bStart.Format(time.RFC3339),
			End:    bEnd.Format(time.RFC3339),
			Status: "no_data",
		}
		if pt := bucketMap[bStart.Unix()]; pt != nil {
			st := mapSiteHealthStatus(pt.Health.ErrorRate, pt.Health.TTFT)
			item.Status = st
			if st != "no_data" {
				item.Uptime = roundSitePercent(pt.Metrics.SuccessRate)
				item.TTFTMs = cloneSiteTTFT(pt.Metrics.TTFT.P50Ms)
			}
		}
		buckets = append(buckets, item)
	}

	groupStatus := "no_data"
	var uptime24h *float64
	var ttftMs *int64
	if row != nil {
		groupStatus = mapSiteHealthStatus(row.Health.ErrorRate, row.Health.TTFT)
		if groupStatus != "no_data" {
			uptime24h = roundSitePercent(row.Metrics.SuccessRate)
			ttftMs = cloneSiteTTFT(row.Metrics.TTFT.P50Ms)
		}
	}

	return siteStatusGroup{
		ID:        id,
		Platform:  platform,
		Name:      name,
		Status:    groupStatus,
		Uptime24h: uptime24h,
		TTFTMs:    ttftMs,
		Buckets:   buckets,
	}
}

// mapSiteHealthStatus 仅由错误率与首字延迟两档决定状态，取较差项，绝不混入缓存命中率。
func mapSiteHealthStatus(errorRateBand, ttftBand string) string {
	worst := siteHealthRank(errorRateBand)
	if r := siteHealthRank(ttftBand); r > worst {
		worst = r
	}
	switch worst {
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
