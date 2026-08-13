//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSettingService_GetCompactPublicSettings_RemovesLargeInlineAssets(t *testing.T) {
	documents := `[{"id":"terms","title":"Terms","content_md":"large legal body"}]`
	logoBytes := []byte("\xff\xd8\xff test jpeg")
	logo := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(logoBytes)
	repo := &settingPublicRepoStub{values: map[string]string{
		SettingKeySiteLogo:                logo,
		SettingKeyLoginAgreementDocuments: documents,
	}}
	svc := NewSettingService(repo, &config.Config{})

	compact, err := svc.GetCompactPublicSettings(context.Background())
	require.NoError(t, err)
	require.Regexp(t, `^/api/v1/settings/public/logo/[a-f0-9]{16}$`, compact.SiteLogo)
	require.Equal(t, []LoginAgreementDocument{{ID: "terms", Title: "Terms"}}, compact.LoginAgreementDocuments)

	legacy, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, logo, legacy.SiteLogo)
	require.Equal(t, "large legal body", legacy.LoginAgreementDocuments[0].ContentMD)
}

func TestCompactSiteLogo_PreservesUnsupportedDataURL(t *testing.T) {
	svg := `data:image/svg+xml,%3Csvg%20xmlns='http://www.w3.org/2000/svg'%3E%3C/svg%3E`
	require.Equal(t, svg, compactSiteLogo(svg))
	require.Equal(t, "https://cdn.example.com/logo.svg", compactSiteLogo("https://cdn.example.com/logo.svg"))
}

func TestSettingService_PublicAssetsRequireCurrentRevision(t *testing.T) {
	logoBytes := []byte("\xff\xd8\xff test jpeg")
	logo := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(logoBytes)
	repo := &settingPublicRepoStub{values: map[string]string{
		SettingKeySiteLogo:                logo,
		SettingKeyLoginAgreementUpdatedAt: "2026-08-13",
		SettingKeyLoginAgreementDocuments: `[{"id":"terms","title":"Terms","content_md":"body"}]`,
	}}
	svc := NewSettingService(repo, &config.Config{})
	compact, err := svc.GetCompactPublicSettings(context.Background())
	require.NoError(t, err)

	logoRevision := compact.SiteLogo[strings.LastIndex(compact.SiteLogo, "/")+1:]
	contentType, data, ok, err := svc.GetPublicLogoAsset(context.Background(), logoRevision)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "image/jpeg", contentType)
	require.Equal(t, logoBytes, data)

	_, _, ok, err = svc.GetPublicLogoAsset(context.Background(), "stale")
	require.NoError(t, err)
	require.False(t, ok)

	doc, ok, err := svc.GetPublicLoginAgreementDocument(context.Background(), compact.LoginAgreementRevision, "terms")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "body", doc.ContentMD)

	_, ok, err = svc.GetPublicLoginAgreementDocument(context.Background(), "stale", "terms")
	require.NoError(t, err)
	require.False(t, ok)
}

type settingPublicRepoStub struct {
	values map[string]string
	err    error
	delay  time.Duration
	calls  atomic.Int32
	mu     sync.RWMutex
}

func (s *settingPublicRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *settingPublicRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *settingPublicRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingPublicRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	s.calls.Add(1)
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func TestSettingService_GetPublicSettings_CachesAndReturnsIndependentCopies(t *testing.T) {
	repo := &settingPublicRepoStub{values: map[string]string{
		SettingKeyRegistrationEmailSuffixWhitelist: `["@example.com"]`,
		SettingKeyTablePageSizeOptions:             `[20,50]`,
	}}
	svc := NewSettingService(repo, &config.Config{})

	first, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	first.RegistrationEmailSuffixWhitelist[0] = "@mutated.invalid"
	first.TablePageSizeOptions[0] = 999

	second, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"@example.com"}, second.RegistrationEmailSuffixWhitelist)
	require.Equal(t, []int{20, 50}, second.TablePageSizeOptions)
	require.EqualValues(t, 1, repo.calls.Load())
}

func TestSettingService_GetPublicSettings_CoalescesConcurrentLoads(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{SettingKeySiteName: "Concurrent Site"},
		delay:  50 * time.Millisecond,
	}
	svc := NewSettingService(repo, &config.Config{})

	const callers = 64
	start := make(chan struct{})
	errs := make(chan error, callers)
	var wait sync.WaitGroup
	for i := 0; i < callers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			settings, err := svc.GetPublicSettings(context.Background())
			if err == nil && settings.SiteName != "Concurrent Site" {
				err = fmt.Errorf("site name = %q", settings.SiteName)
			}
			errs <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, repo.calls.Load())
}

func TestSettingService_GetPublicSettings_InvalidatesAfterSettingsRefresh(t *testing.T) {
	repo := &settingPublicRepoStub{values: map[string]string{
		SettingKeyRegistrationEnabled: "true",
	}}
	svc := NewSettingService(repo, &config.Config{})

	before, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, before.RegistrationEnabled)

	repo.mu.Lock()
	repo.values[SettingKeyRegistrationEnabled] = "false"
	repo.mu.Unlock()
	svc.refreshCachedSettings(&SystemSettings{})

	after, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.False(t, after.RegistrationEnabled)
	require.EqualValues(t, 2, repo.calls.Load())
}

func (s *settingPublicRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *settingPublicRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *settingPublicRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestSettingService_GetPublicSettings_ExposesRegistrationEmailSuffixWhitelist(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyRegistrationEnabled:              "true",
			SettingKeyEmailVerifyEnabled:               "true",
			SettingKeyRegistrationEmailSuffixWhitelist: `["@EXAMPLE.com"," @foo.bar ","*.EDU.CN","@invalid_domain",""]`,
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"@example.com", "@foo.bar", "*.edu.cn"}, settings.RegistrationEmailSuffixWhitelist)
}

func TestSettingService_GetPublicSettings_ExposesTablePreferences(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyTableDefaultPageSize: "50",
			SettingKeyTablePageSizeOptions: "[20,50,100]",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 50, settings.TableDefaultPageSize)
	require.Equal(t, []int{20, 50, 100}, settings.TablePageSizeOptions)
}

func TestSettingService_GetPublicSettings_ExposesCompactHomeEnabled(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyCompactHomeEnabled: "true",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())

	require.NoError(t, err)
	require.True(t, settings.CompactHomeEnabled)

	missingSettings, err := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{}).
		GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.False(t, missingSettings.CompactHomeEnabled)
}

func TestSettingService_ChannelMonitorHideThroughputDefaultsToPrivate(t *testing.T) {
	missing := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{}).GetChannelMonitorRuntime(context.Background())
	require.True(t, missing.HideThroughput)
	public, err := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{}).GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, public.ChannelMonitorHideThroughput)

	for _, value := range []string{"false", "0", "off", "disabled"} {
		runtime := NewSettingService(&settingPublicRepoStub{values: map[string]string{
			SettingKeyChannelMonitorHideThroughput: value,
		}}, &config.Config{}).GetChannelMonitorRuntime(context.Background())
		require.False(t, runtime.HideThroughput, "value=%q", value)
	}
}

func TestSettingService_GetPublicSettings_ExposesForceEmailOnThirdPartySignup(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyForceEmailOnThirdPartySignup: "true",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.ForceEmailOnThirdPartySignup)
}

func TestSettingService_GetPublicSettings_ExposesAllowUserViewErrorRequests(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyAllowUserViewErrorRequests: "true",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.AllowUserViewErrorRequests)
}

func TestSettingService_GetPublicSettings_ExposesWeChatOAuthModeCapabilities(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{
		values: map[string]string{
			SettingKeyWeChatConnectEnabled:             "true",
			SettingKeyWeChatConnectAppID:               "wx-mp-app",
			SettingKeyWeChatConnectAppSecret:           "wx-mp-secret",
			SettingKeyWeChatConnectMode:                "mp",
			SettingKeyWeChatConnectScopes:              "snsapi_base",
			SettingKeyWeChatConnectOpenEnabled:         "true",
			SettingKeyWeChatConnectMPEnabled:           "true",
			SettingKeyWeChatConnectRedirectURL:         "https://api.example.com/api/v1/auth/oauth/wechat/callback",
			SettingKeyWeChatConnectFrontendRedirectURL: "/auth/wechat/callback",
		},
	}, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.WeChatOAuthEnabled)
	require.True(t, settings.WeChatOAuthOpenEnabled)
	require.True(t, settings.WeChatOAuthMPEnabled)
}

func TestSettingService_GetPublicSettings_DoesNotExposeMobileOnlyWeChatAsWebOAuthAvailable(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{
		values: map[string]string{
			SettingKeyWeChatConnectEnabled:             "true",
			SettingKeyWeChatConnectMobileEnabled:       "true",
			SettingKeyWeChatConnectMode:                "mobile",
			SettingKeyWeChatConnectMobileAppID:         "wx-mobile-app",
			SettingKeyWeChatConnectMobileAppSecret:     "wx-mobile-secret",
			SettingKeyWeChatConnectFrontendRedirectURL: "/auth/wechat/callback",
		},
	}, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.WeChatOAuthEnabled)
	require.False(t, settings.WeChatOAuthOpenEnabled)
	require.False(t, settings.WeChatOAuthMPEnabled)
	require.True(t, settings.WeChatOAuthMobileEnabled)
}

func TestSettingService_GetPublicSettings_FallsBackToConfigForWeChatOAuthCapabilities(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{
		WeChat: config.WeChatConnectConfig{
			Enabled:             true,
			OpenEnabled:         true,
			OpenAppID:           "wx-open-config",
			OpenAppSecret:       "wx-open-secret",
			FrontendRedirectURL: "/auth/wechat/config-callback",
		},
	})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.WeChatOAuthEnabled)
	require.True(t, settings.WeChatOAuthOpenEnabled)
	require.False(t, settings.WeChatOAuthMPEnabled)
	require.False(t, settings.WeChatOAuthMobileEnabled)
}
