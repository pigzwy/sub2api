package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func handlerCatalogPrice(value float64) *float64 { return &value }

func modelPricingTestGroup(id int64, exclusive bool, model string, price float64) service.Group {
	return service.Group{
		ID:               id,
		Name:             "sensitive-group-name",
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		Platform:         service.PlatformComposite,
		IsExclusive:      exclusive,
		RateMultiplier:   1,
		ModelPricing: []service.ChannelModelPricing{{
			Models:          []string{model},
			BillingMode:     service.BillingModeImage,
			PerRequestPrice: handlerCatalogPrice(price),
		}},
	}
}

func newKeyModelPricingContext(apiKey *service.APIKey) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/sub2api/model-pricing", nil)
	if apiKey != nil {
		c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	}
	return c, w
}

func TestGatewayHandlerKeyModelPricingReturnsEffectivePricesOnly(t *testing.T) {
	group := modelPricingTestGroup(71, false, "gpt-image-2", 0.2)
	group.ImageRateIndependent = true
	group.ImageRateMultiplier = 1.5
	group.ModelPricing = append(group.ModelPricing, service.ChannelModelPricing{
		Models:          []string{"grok-imagine-video"},
		BillingMode:     service.BillingModeVideo,
		PerRequestPrice: handlerCatalogPrice(0.04),
	}, service.ChannelModelPricing{
		Models:      []string{"text-model"},
		BillingMode: service.BillingModeToken,
	}, service.ChannelModelPricing{
		Models:          []string{"legacy-request-model"},
		BillingMode:     service.BillingModePerRequest,
		PerRequestPrice: handlerCatalogPrice(0.5),
	})
	groupID := group.ID
	apiKey := &service.APIKey{ID: 10, UserID: 20, Key: "sk-sensitive-value", GroupID: &groupID, Group: &group}
	c, w := newKeyModelPricingContext(apiKey)

	newKeyBillingHandler(nil).KeyModelPricing(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var got groupModelPricingResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, "sub2api.group_model_pricing", got.Object)
	require.Equal(t, groupModelPricingSchemaVersion, got.SchemaVersion)
	require.Equal(t, int64(71), got.GroupID)
	require.Equal(t, "USD", got.Currency)
	require.Equal(t, "effective", got.PriceScope)
	require.True(t, got.PricesIncludeMultiplier)
	require.Equal(t, 1.5, got.Multipliers.Image)
	require.Equal(t, 1.0, got.Multipliers.Video)
	require.InDelta(t, 0.3, got.Models["gpt-image-2"].Prices["2K"], 1e-12)
	require.Equal(t, 0.04, got.Models["grok-imagine-video"].Prices["720p"])
	require.True(t, got.Models["gpt-image-2"].Displayable)
	require.False(t, got.Models["text-model"].Displayable)
	require.False(t, got.Models["legacy-request-model"].Displayable)
	var raw struct {
		Models map[string]map[string]json.RawMessage `json:"models"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	for _, model := range []string{"text-model", "legacy-request-model"} {
		require.NotContains(t, raw.Models[model], "unit")
		require.NotContains(t, raw.Models[model], "prices")
	}
	require.NotContains(t, w.Body.String(), apiKey.Key)
	require.NotContains(t, w.Body.String(), group.Name)
	require.NotContains(t, w.Body.String(), "channel")
	require.NotContains(t, w.Body.String(), "account")
}

func TestGatewayHandlerKeyModelPricingUsesUserOverrideAndEmptyIsExplicit(t *testing.T) {
	group := modelPricingTestGroup(71, false, "gpt-image-2", 0.2)
	group.ModelPricing = nil
	groupID := group.ID
	userRate := 0.5
	apiKey := &service.APIKey{UserID: 20, GroupID: &groupID, Group: &group}
	repo := &keyBillingUserGroupRateRepo{rate: &userRate}
	c, w := newKeyModelPricingContext(apiKey)

	newKeyBillingHandler(repo).KeyModelPricing(c)

	require.Equal(t, http.StatusOK, w.Code)
	var got groupModelPricingResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, 0.5, got.Multipliers.ResolvedGroup)
	require.NotNil(t, got.Models)
	require.Empty(t, got.Models)
}

func TestGatewayHandlerKeyModelPricingRejectsMissingOrMismatchedGroup(t *testing.T) {
	t.Run("missing API key", func(t *testing.T) {
		c, w := newKeyModelPricingContext(nil)
		newKeyBillingHandler(nil).KeyModelPricing(c)
		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	})

	t.Run("typed nil API key", func(t *testing.T) {
		c, w := newKeyModelPricingContext(nil)
		c.Set(string(middleware2.ContextKeyAPIKey), (*service.APIKey)(nil))
		newKeyBillingHandler(nil).KeyModelPricing(c)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("ungrouped API key", func(t *testing.T) {
		c, w := newKeyModelPricingContext(&service.APIKey{})
		newKeyBillingHandler(nil).KeyModelPricing(c)
		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("mismatched group snapshot", func(t *testing.T) {
		groupID := int64(71)
		c, w := newKeyModelPricingContext(&service.APIKey{GroupID: &groupID, Group: &service.Group{ID: 72}})
		newKeyBillingHandler(nil).KeyModelPricing(c)
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

type modelPricingUserRepo struct {
	service.UserRepository
	user *service.User
}

func (r *modelPricingUserRepo) GetByID(context.Context, int64) (*service.User, error) {
	return r.user, nil
}

type modelPricingGroupRepo struct {
	service.GroupRepository
	groups []service.Group
}

func (r *modelPricingGroupRepo) ListActive(context.Context) ([]service.Group, error) {
	return r.groups, nil
}

type modelPricingSubscriptionRepo struct {
	service.UserSubscriptionRepository
}

func (r *modelPricingSubscriptionRepo) ListActiveByUserID(context.Context, int64) ([]service.UserSubscription, error) {
	return nil, nil
}

type modelPricingRateRepo struct {
	service.UserGroupRateRepository
	rates map[int64]float64
}

func (r *modelPricingRateRepo) GetByUserID(context.Context, int64) (map[int64]float64, error) {
	return r.rates, nil
}

func newJWTModelPricingHandler(groups []service.Group, rates map[int64]float64) *APIKeyHandler {
	svc := service.NewAPIKeyService(
		nil,
		&modelPricingUserRepo{user: &service.User{ID: 20, Status: service.StatusActive}},
		&modelPricingGroupRepo{groups: groups},
		&modelPricingSubscriptionRepo{},
		&modelPricingRateRepo{rates: rates},
		nil,
		nil,
	)
	return NewAPIKeyHandler(svc)
}

func newJWTModelPricingContext(groupID string, authenticated bool) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/groups/"+groupID+"/model-pricing", nil)
	c.Params = gin.Params{{Key: "id", Value: groupID}}
	if authenticated {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 20})
	}
	return c, w
}

func TestAPIKeyHandlerGroupModelPricingEnforcesJWTGroupAccess(t *testing.T) {
	allowed := modelPricingTestGroup(71, false, "gpt-image-2", 0.2)
	hidden := modelPricingTestGroup(72, true, "secret-model", 99)
	h := newJWTModelPricingHandler([]service.Group{allowed, hidden}, map[int64]float64{71: 0.5, 72: 9})

	t.Run("allowed group uses current user rate", func(t *testing.T) {
		c, w := newJWTModelPricingContext("71", true)
		h.GetGroupModelPricing(c)
		require.Equal(t, http.StatusOK, w.Code)
		var got groupModelPricingResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		require.Equal(t, 0.5, got.Multipliers.ResolvedGroup)
		require.Equal(t, 0.1, got.Models["gpt-image-2"].Prices["2K"])
	})

	t.Run("exclusive group is not disclosed", func(t *testing.T) {
		c, w := newJWTModelPricingContext("72", true)
		h.GetGroupModelPricing(c)
		require.Equal(t, http.StatusNotFound, w.Code)
		require.NotContains(t, w.Body.String(), "secret-model")
		require.NotContains(t, w.Body.String(), "99")
	})

	t.Run("authentication is required", func(t *testing.T) {
		c, w := newJWTModelPricingContext("71", false)
		h.GetGroupModelPricing(c)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("service is required", func(t *testing.T) {
		c, w := newJWTModelPricingContext("71", true)
		NewAPIKeyHandler(nil).GetGroupModelPricing(c)
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	})
}
