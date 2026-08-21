package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const groupModelPricingSchemaVersion = 1

const maxRequestedAudioPricingModels = 32

const maxRequestedAudioPricingListBytes = 8192

type groupModelAudioTokenPricingResolver interface {
	AddEffectiveAudioTokenModelPricing(
		ctx context.Context,
		catalog *service.EffectiveGroupModelPricingCatalog,
		group *service.Group,
		model string,
		tokenMultiplier float64,
		pricingAt time.Time,
	) bool
}

type groupModelPricingMultipliersResponse struct {
	ResolvedGroup float64 `json:"resolved_group"`
	Token         float64 `json:"token"`
	Image         float64 `json:"image"`
	Video         float64 `json:"video"`
}

type groupAudioPriceResponse struct {
	Unit   string             `json:"unit"`
	Source string             `json:"source"`
	Prices map[string]float64 `json:"prices"`
}

type groupModelPriceResponse struct {
	MatchType   string              `json:"match_type"`
	Priority    *int                `json:"priority,omitempty"`
	BillingMode service.BillingMode `json:"billing_mode"`
	Displayable bool                `json:"displayable"`
	Unit        string              `json:"unit,omitempty"`
	Prices      map[string]float64  `json:"prices,omitempty"`
}

type groupModelPricingResponse struct {
	Object                  string                               `json:"object"`
	SchemaVersion           int                                  `json:"schema_version"`
	GroupID                 int64                                `json:"group_id"`
	Currency                string                               `json:"currency"`
	PriceScope              string                               `json:"price_scope"`
	PricesIncludeMultiplier bool                                 `json:"prices_include_multiplier"`
	Multipliers             groupModelPricingMultipliersResponse `json:"multipliers"`
	Models                  map[string]groupModelPriceResponse   `json:"models"`
	Audio                   map[string]groupAudioPriceResponse   `json:"audio"`
	ObservedAt              time.Time                            `json:"observed_at"`
}

// KeyModelPricing returns effective media and audio prices for the group bound
// to the authenticated API key. GET /v1/sub2api/model-pricing
func (h *GatewayHandler) KeyModelPricing(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if h.cfg != nil && h.cfg.RunMode == config.RunModeSimple {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Model pricing is not supported in simple mode")
		return
	}
	if apiKey.GroupID == nil {
		h.errorResponse(c, http.StatusForbidden, "permission_error", "API key is not assigned to a group")
		return
	}
	if apiKey.Group == nil || apiKey.Group.ID != *apiKey.GroupID {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Model pricing is unavailable")
		return
	}
	resolvedRate, ok := h.resolveKeyBillingRate(c, apiKey)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "Model pricing is unavailable")
		return
	}

	pricingAt := timezone.Now()
	c.JSON(http.StatusOK, buildGroupModelPricingResponse(
		c.Request.Context(), apiKey.Group, resolvedRate, h.modelPricingResolver,
		requestedAudioPricingModels(c), pricingAt,
	))
}

// GetGroupModelPricing returns the same contract for a JWT-authenticated user,
// after applying the existing active-group, exclusive-group, and subscription checks.
// GET /api/v1/groups/:id/model-pricing
func (h *APIKeyHandler) GetGroupModelPricing(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	groupID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || groupID <= 0 {
		response.BadRequest(c, "Invalid group ID")
		return
	}
	if h.apiKeyService == nil {
		response.InternalError(c, "Group model pricing is unavailable")
		return
	}

	groups, err := h.apiKeyService.GetAvailableGroups(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	var selected *service.Group
	for i := range groups {
		if groups[i].ID == groupID {
			selected = &groups[i]
			break
		}
	}
	if selected == nil {
		response.NotFound(c, "Group model pricing not found")
		return
	}

	resolvedRate := selected.RateMultiplier
	rates, err := h.apiKeyService.GetUserGroupRates(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if userRate, exists := rates[groupID]; exists {
		resolvedRate = userRate
	}

	pricingAt := timezone.Now()
	c.JSON(http.StatusOK, buildGroupModelPricingResponse(
		c.Request.Context(), selected, resolvedRate, h.modelPricingResolver,
		requestedAudioPricingModels(c), pricingAt,
	))
}

func buildGroupModelPricingResponse(
	ctx context.Context,
	group *service.Group,
	resolvedRate float64,
	resolver groupModelAudioTokenPricingResolver,
	audioModels []string,
	pricingAt time.Time,
) groupModelPricingResponse {
	catalog := service.BuildEffectiveGroupModelPricingCatalog(group, resolvedRate)
	baseMultiplier := catalog.ResolvedGroupMultiplier
	tokenMultiplier := catalog.ResolvedGroupMultiplier
	if group != nil {
		tokenMultiplier *= group.PeakMultiplierAt(pricingAt)
	}
	if tokenMultiplier < 0 {
		tokenMultiplier = 0
	}
	catalog.TokenMultiplier = tokenMultiplier
	service.AddEffectiveGroupAudioPricing(&catalog, group, baseMultiplier)
	if resolver != nil {
		for _, model := range audioModels {
			resolver.AddEffectiveAudioTokenModelPricing(ctx, &catalog, group, model, tokenMultiplier, pricingAt)
		}
	}
	models := make(map[string]groupModelPriceResponse, len(catalog.Models))
	for model, pricing := range catalog.Models {
		models[model] = groupModelPriceResponse{
			MatchType:   pricing.MatchType,
			Priority:    pricing.Priority,
			BillingMode: pricing.BillingMode,
			Displayable: pricing.Displayable,
			Unit:        pricing.Unit,
			Prices:      pricing.Prices,
		}
	}
	audio := make(map[string]groupAudioPriceResponse, len(catalog.Audio))
	for mode, pricing := range catalog.Audio {
		audio[mode] = groupAudioPriceResponse{
			Unit:   pricing.Unit,
			Source: pricing.Source,
			Prices: pricing.Prices,
		}
	}
	groupID := int64(0)
	if group != nil {
		groupID = group.ID
	}
	return groupModelPricingResponse{
		Object:                  "sub2api.group_model_pricing",
		SchemaVersion:           groupModelPricingSchemaVersion,
		GroupID:                 groupID,
		Currency:                "USD",
		PriceScope:              "effective",
		PricesIncludeMultiplier: true,
		Multipliers: groupModelPricingMultipliersResponse{
			ResolvedGroup: catalog.ResolvedGroupMultiplier,
			Token:         catalog.TokenMultiplier,
			Image:         catalog.ImageMultiplier,
			Video:         catalog.VideoMultiplier,
		},
		Models:     models,
		Audio:      audio,
		ObservedAt: pricingAt.UTC(),
	}
}

func requestedAudioPricingModels(c *gin.Context) []string {
	seen := make(map[string]struct{})
	models := make([]string, 0, maxRequestedAudioPricingModels)
	add := func(value string) {
		if len(models) >= maxRequestedAudioPricingModels {
			return
		}
		model := strings.ToLower(strings.TrimSpace(value))
		if model == "" || len(model) > 200 || !strings.Contains(model, "realtime") || strings.Contains(model, "*") {
			return
		}
		if _, exists := seen[model]; exists {
			return
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	for _, model := range openai.DefaultModels {
		if strings.HasPrefix(strings.ToLower(model.ID), "gpt-realtime") {
			add(model.ID)
		}
	}
	if c != nil {
		for _, model := range c.QueryArray("model") {
			if len(models) >= maxRequestedAudioPricingModels {
				break
			}
			add(model)
		}
		for _, raw := range c.QueryArray("models") {
			if len(models) >= maxRequestedAudioPricingModels {
				break
			}
			if len(raw) > maxRequestedAudioPricingListBytes {
				continue
			}
			for raw != "" && len(models) < maxRequestedAudioPricingModels {
				model, rest, found := strings.Cut(raw, ",")
				add(model)
				if !found {
					break
				}
				raw = rest
			}
		}
	}
	return models
}
