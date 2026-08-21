package handler

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const groupModelPricingSchemaVersion = 1

type groupModelPricingMultipliersResponse struct {
	ResolvedGroup float64 `json:"resolved_group"`
	Image         float64 `json:"image"`
	Video         float64 `json:"video"`
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
}

// KeyModelPricing returns the effective explicit media prices for the group
// bound to the authenticated API key. GET /v1/sub2api/model-pricing
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

	c.JSON(http.StatusOK, buildGroupModelPricingResponse(apiKey.Group, resolvedRate))
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

	c.JSON(http.StatusOK, buildGroupModelPricingResponse(selected, resolvedRate))
}

func buildGroupModelPricingResponse(group *service.Group, resolvedRate float64) groupModelPricingResponse {
	catalog := service.BuildEffectiveGroupModelPricingCatalog(group, resolvedRate)
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
			Image:         catalog.ImageMultiplier,
			Video:         catalog.VideoMultiplier,
		},
		Models: models,
	}
}
