package handler

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	siteSnapshotBucketDuration      = 30 * time.Minute
	siteSnapshotBucketMinutes       = 30
	siteSnapshotBucketCount         = 48
	siteSnapshotCurrentLookbackBins = 3 // 分组当前状态看最近 3 个已结束的半小时桶

	// 请求日志是异步落库的，会比请求本身晚一点。半点刚过就算出来的那份，最后一个桶可能还缺几条，
	// 所以过了这段时间后允许再算一次；此后同一个半小时内不再重算。
	siteSnapshotSettleDelay = 5 * time.Minute
	// 一次计算的上限。计算用的是脱离请求的上下文：触发它的那个访客断开了，也不该让其他等待的人落空。
	siteSnapshotComputeTimeout = 25 * time.Second

	// 判定规则。数据是被动的（统计真实请求，不主动探测），所以样本量决定能下多重的结论：
	//   - 一个桶里请求不少于 siteSnapshotFullSample：错误率到 siteSnapshotOutageRate 报故障，
	//     到 siteSnapshotDegradedRate 报波动；
	//   - 介于 siteSnapshotLowSampleFloor 和 siteSnapshotFullSample 之间：错误率到
	//     siteSnapshotOutageRate 也只报波动，样本太少，不足以下“故障”的结论；
	//   - 少于 siteSnapshotLowSampleFloor：当作没有流量，按正常算。
	// 首字延迟只展示、不参与判定：图像、视频、推理模型的首字本来就慢，拿它判状态会长期误报。
	siteSnapshotFullSample     = 20
	siteSnapshotLowSampleFloor = 5
	siteSnapshotDegradedRate   = 0.05
	siteSnapshotOutageRate     = 0.20

	siteStatusNormal   = "normal"
	siteStatusDegraded = "degraded"
	siteStatusOutage   = "outage"
)

type siteSnapshotPlazaLister interface {
	ListGroups(ctx context.Context) ([]service.PlazaGroup, error)
}

// SiteSnapshotHandler 为官网状态页提供脱敏后的公开分组健康状态。
//
// 数据直接统计请求日志（usage_logs）与错误日志（ops_error_logs），不依赖渠道监控
// （主动探测的 v1 和被动聚合的 v2 都不用）：那两套随后台开关启停、口径也可在后台调整，
// 官网对外展示的状态不应跟着变。错误日志由运维监控写入，关掉它这里就看不到失败。
//
// 官网经反向代理直接转发访客请求过来，所以这里自带缓存：同一个半小时窗口内最多计算两次，
// 其余请求一律返回内存里那份。被刷的代价约等于返回一段现成的字节。
// 只输出严格白名单字段，绝不暴露请求数、令牌数、用户或账号信息。
type SiteSnapshotHandler struct {
	plazaService siteSnapshotPlazaLister
	statusRepo   service.SiteStatusRepository
	now          func() time.Time

	mu    sync.Mutex
	cache *siteStatusCache
}

// siteStatusCache 是某个半小时窗口算好的结果，按界面语言各存一份序列化后的响应体。
type siteStatusCache struct {
	windowEnd  time.Time
	computedAt time.Time
	bodies     map[string][]byte
}

// NewSiteSnapshotHandler 创建官网状态 handler。
func NewSiteSnapshotHandler(
	plazaService *service.ModelPlazaService,
	statusRepo service.SiteStatusRepository,
) *SiteSnapshotHandler {
	return &SiteSnapshotHandler{
		plazaService: plazaService,
		statusRepo:   statusRepo,
		now:          time.Now,
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

// siteStatusLocales 是官网支持的界面语言。请求语言先归一到这两种之一再查缓存，
// 否则任意 Accept-Language 值都会产生一份新缓存，等于给了绕过缓存的办法。
var siteStatusLocales = []string{"zh-CN", "en-US"}

func normalizeSiteLocale(c *gin.Context) string {
	raw := strings.TrimSpace(c.Query("lang"))
	if raw == "" {
		raw = strings.TrimSpace(c.GetHeader("Accept-Language"))
	}
	if strings.HasPrefix(strings.ToLower(raw), "en") {
		return "en-US"
	}
	return "zh-CN"
}

// GetStatus 返回最近 24 小时（48 个已结束的 30 分钟桶）的公开分组脱敏健康状态。
// GET /api/v1/internal/site/status
func (h *SiteSnapshotHandler) GetStatus(c *gin.Context) {
	c.Writer.Header().Add("Vary", "Accept-Language")

	if h == nil || h.plazaService == nil || h.statusRepo == nil {
		c.Header("Cache-Control", "no-store")
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	body, err := h.snapshotBody(c.Request.Context(), normalizeSiteLocale(c))
	if err != nil {
		c.Header("Cache-Control", "no-store")
		response.ErrorFrom(c, err)
		return
	}
	// 浏览器端只缓存一分钟：页面在半点过后几分钟刷新，缓存太久会让刷新读到上一份。
	c.Header("Cache-Control", "public, max-age=60")
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// snapshotBody 返回 locale 对应的响应体；必要时重算，算不出来就继续用上一份。
func (h *SiteSnapshotHandler) snapshotBody(ctx context.Context, locale string) ([]byte, error) {
	nowFn := h.now
	if nowFn == nil {
		nowFn = time.Now
	}

	// 计算期间一直持锁：并发请求排队等同一次计算的结果，而不是各算各的。
	h.mu.Lock()
	defer h.mu.Unlock()

	now := nowFn().UTC()
	windowEnd := now.Truncate(siteSnapshotBucketDuration)
	if siteStatusCacheFresh(h.cache, windowEnd, now) {
		return h.cache.bodies[locale], nil
	}

	computeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), siteSnapshotComputeTimeout)
	defer cancel()
	fresh, err := h.compute(computeCtx, windowEnd, now)
	if err != nil {
		if h.cache != nil {
			// 这次没算出来：继续给上一份。它的 updated_at 是当时的时间，官网据此判断数据是否过期。
			return h.cache.bodies[locale], nil
		}
		return nil, err
	}
	h.cache = fresh
	return fresh.bodies[locale], nil
}

// siteStatusCacheFresh 判断缓存能否直接用：必须是当前窗口的，并且要么是在日志落稳之后算的，
// 要么现在还没到那个时间（此时重算也不会更准）。
func siteStatusCacheFresh(cache *siteStatusCache, windowEnd, now time.Time) bool {
	if cache == nil || !cache.windowEnd.Equal(windowEnd) {
		return false
	}
	settled := windowEnd.Add(siteSnapshotSettleDelay)
	return !cache.computedAt.Before(settled) || now.Before(settled)
}

func (h *SiteSnapshotHandler) compute(ctx context.Context, windowEnd, now time.Time) (*siteStatusCache, error) {
	rawGroups, err := h.plazaService.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	// 与匿名模型广场同一套可见规则：只有非专属分组，状态页和价格页的分组永远一致。
	visible := filterPlazaVisibleGroups(rawGroups, nil, false)
	windowStart := windowEnd.Add(-siteSnapshotBucketCount * siteSnapshotBucketDuration)

	statsByGroup := make(map[int64][]service.SiteStatusStat, len(visible))
	if len(visible) > 0 {
		groupIDs := make([]int64, 0, len(visible))
		for i := range visible {
			groupIDs = append(groupIDs, visible[i].ID)
		}
		stats, err := h.statusRepo.GroupStats(ctx, groupIDs, windowStart, windowEnd, siteSnapshotBucketDuration)
		if err != nil {
			return nil, err
		}
		for _, st := range stats {
			statsByGroup[st.GroupID] = append(statsByGroup[st.GroupID], st)
		}
	}

	built := make([]siteStatusGroup, 0, len(visible))
	for i := range visible {
		g := &visible[i]
		built = append(built, buildSiteStatusGroup(g.ID, g.Platform, g.Name, statsByGroup[g.ID], windowStart))
	}

	out := &siteStatusCache{windowEnd: windowEnd, computedAt: now, bodies: make(map[string][]byte, len(siteStatusLocales))}
	for _, locale := range siteStatusLocales {
		groups := make([]siteStatusGroup, len(built))
		copy(groups, built)
		for i := range visible {
			groups[i].Name, _ = visible[i].I18n.Localize(locale, visible[i].Name, visible[i].Description)
		}
		body, err := json.Marshal(siteStatusResponse{
			OK:            true,
			UpdatedAt:     now.Format(time.RFC3339),
			BucketMinutes: siteSnapshotBucketMinutes,
			Groups:        groups,
		})
		if err != nil {
			return nil, err
		}
		out.bodies[locale] = body
	}
	return out, nil
}

func buildSiteStatusGroup(
	id int64,
	platform string,
	name string,
	stats []service.SiteStatusStat,
	windowStart time.Time,
) siteStatusGroup {
	byBucket := make(map[int64]service.SiteStatusStat, len(stats))
	var total service.SiteStatusStat
	for _, st := range stats {
		if st.Total {
			total = st
			continue
		}
		byBucket[st.BucketStart.UTC().Truncate(siteSnapshotBucketDuration).Unix()] = st
	}

	buckets := make([]siteStatusBucket, 0, siteSnapshotBucketCount)
	observed := make([]bool, 0, siteSnapshotBucketCount)
	for i := 0; i < siteSnapshotBucketCount; i++ {
		bStart := windowStart.Add(time.Duration(i) * siteSnapshotBucketDuration)
		st := byBucket[bStart.Unix()] // 没有请求的桶取到零值
		status, seen := siteStatStatus(st)
		observed = append(observed, seen)
		item := siteStatusBucket{
			Start:  bStart.Format(time.RFC3339),
			End:    bStart.Add(siteSnapshotBucketDuration).Format(time.RFC3339),
			Status: status,
		}
		// 请求太少的桶按正常画，也就不报可用率：两三个请求算出来的百分比没有意义，
		// 还会出现“状态正常、可用率 0%”这种自相矛盾的提示。
		if seen {
			item.Uptime = siteStatUptime(st)
			item.TTFTMs = cloneSiteTTFT(st.TTFTP50Ms)
		}
		buckets = append(buckets, item)
	}

	// 可用率与首字延迟是 24 小时汇总；当前状态只看最近几个桶，
	// 避免刚发生的故障被前 23 小时稀释，或很久以前的波动一直挂着。
	return siteStatusGroup{
		ID:        id,
		Platform:  platform,
		Name:      name,
		Status:    resolveCurrentSiteStatus(buckets, observed, total, siteSnapshotCurrentLookbackBins),
		Uptime24h: siteStatUptime(total),
		TTFTMs:    cloneSiteTTFT(total.TTFTP50Ms),
		Buckets:   buckets,
	}
}

// siteStatStatus 把一个桶的请求统计映射成对外状态，规则见文件顶部的常量说明。
// observed 表示这个结论有真实流量支撑，而不是“没有流量所以按正常算”。
func siteStatStatus(st service.SiteStatusStat) (status string, observed bool) {
	total := st.Success + st.Errors
	if total < siteSnapshotLowSampleFloor {
		return siteStatusNormal, false
	}
	rate := float64(st.Errors) / float64(total)
	switch {
	case rate >= siteSnapshotOutageRate && total >= siteSnapshotFullSample:
		return siteStatusOutage, true
	case rate >= siteSnapshotOutageRate:
		return siteStatusDegraded, true
	case rate >= siteSnapshotDegradedRate && total >= siteSnapshotFullSample:
		return siteStatusDegraded, true
	default:
		return siteStatusNormal, true
	}
}

// siteStatUptime 返回可用率百分数：成功数 ÷（成功数 + 服务侧失败数）。
// 用户自身原因的失败（参数错误、余额不足、主动取消等）不在分母里，见 repository.siteStatusErrorSQL。
// 没有请求时没有可用率可报，返回 nil。
func siteStatUptime(st service.SiteStatusStat) *float64 {
	total := st.Success + st.Errors
	if total <= 0 {
		return nil
	}
	return roundSitePercent(float64(st.Success) / float64(total))
}

// resolveCurrentSiteStatus 决定分组的当前状态，按顺序：
//  1. 最近 lookback 个已结束桶里，最新一个有真实流量的桶的状态。
//     不直接取最后一个桶：分组刚出故障时用户往往就不再发请求了，最后一个桶没有流量
//     会被当成正常，正好把故障盖住；往前多看几个桶，能看到故障发生时的那个桶。
//  2. 这几个桶都没什么流量时，看 24 小时汇总。冷门分组每半小时只有零星几个请求，
//     单看哪个桶都“样本不足”，但全天加起来几乎全部失败——这种分组实际上是不可用的，
//     不能因为每个桶都安静就显示正常。
//  3. 24 小时里也没什么请求：没有流量不等于有问题，按正常算。
func resolveCurrentSiteStatus(buckets []siteStatusBucket, observed []bool, total service.SiteStatusStat, lookback int) string {
	start := len(buckets) - lookback
	if start < 0 {
		start = 0
	}
	for i := len(buckets) - 1; i >= start; i-- {
		if i < len(observed) && observed[i] {
			return buckets[i].Status
		}
	}
	status, _ := siteStatStatus(total)
	return status
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
