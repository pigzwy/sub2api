package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func newOpenAISyncUpstreamTestAccount(id int64, priority int, groupID int64, syncDisabled bool) Account {
	extra := map[string]any{}
	if syncDisabled {
		extra[AccountExtraOpenAISyncUpstreamDisabled] = true
	}
	return Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    priority,
		GroupIDs:    []int64{groupID},
		Extra:       extra,
	}
}

func selectOpenAISyncUpstreamTestAccount(svc *OpenAIGatewayService, ctx context.Context, groupID int64, sessionHash string, excluded ...int64) (*AccountSelectionResult, error) {
	var excludedIDs map[int64]struct{}
	if len(excluded) > 0 {
		excludedIDs = make(map[int64]struct{}, len(excluded))
		for _, id := range excluded {
			excludedIDs[id] = struct{}{}
		}
	}
	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		ctx, &groupID, "", sessionHash, "gpt-6.2-sol", excludedIDs,
		OpenAIUpstreamTransportHTTPSSE, OpenAIEndpointCapabilityChatCompletions,
		false, false, false,
	)
	return selection, err
}

func TestAccountAllowsOpenAISyncUpstream(t *testing.T) {
	require.False(t, (*Account)(nil).AllowsOpenAISyncUpstream())
	// 没有这个键的存量账号保持原行为：允许同步请求。
	require.True(t, (&Account{}).AllowsOpenAISyncUpstream())
	require.True(t, (&Account{Extra: map[string]any{}}).AllowsOpenAISyncUpstream())
	require.True(t, (&Account{Extra: map[string]any{AccountExtraOpenAISyncUpstreamDisabled: false}}).AllowsOpenAISyncUpstream())
	require.False(t, (&Account{Extra: map[string]any{AccountExtraOpenAISyncUpstreamDisabled: true}}).AllowsOpenAISyncUpstream())
	// 只认布尔 true；字符串等脏值不应把账号意外挡在同步请求之外。
	require.True(t, (&Account{Extra: map[string]any{AccountExtraOpenAISyncUpstreamDisabled: "true"}}).AllowsOpenAISyncUpstream())
}

func TestOpenAISyncUpstreamVetoReason(t *testing.T) {
	refuses := newOpenAISyncUpstreamTestAccount(1, 0, 1, true)
	allows := newOpenAISyncUpstreamTestAccount(2, 0, 1, false)
	syncCtx := WithOpenAISyncUpstreamRequest(context.Background())

	vetoed, reason := openAISyncUpstreamVetoReason(syncCtx, &refuses)
	require.True(t, vetoed)
	require.Equal(t, "sync_upstream_disabled", reason)

	for name, tc := range map[string]struct {
		ctx     context.Context
		account *Account
	}{
		"sync request, account allows":            {syncCtx, &allows},
		"streaming request, account refuses sync": {context.Background(), &refuses},
		"nil context": {nil, &refuses},
		"nil account": {syncCtx, nil},
	} {
		vetoed, reason := openAISyncUpstreamVetoReason(tc.ctx, tc.account)
		require.False(t, vetoed, name)
		require.Empty(t, reason, name)
	}

	require.False(t, isOpenAISyncUpstreamRequest(context.Background()))
	require.True(t, isOpenAISyncUpstreamRequest(WithOpenAISyncUpstreamRequest(nil)))
}

// legacy 引擎与 DB recheck 共用的资格判定覆盖同步请求开关。
func TestOpenAISyncUpstream_EligibilityFunctionVetoes(t *testing.T) {
	refuses := newOpenAISyncUpstreamTestAccount(11, 0, 1, true)
	allows := newOpenAISyncUpstreamTestAccount(12, 0, 1, false)
	syncCtx := WithOpenAISyncUpstreamRequest(context.Background())

	require.Equal(t, "sync_upstream_disabled",
		openAICompatibleAccountEligibilityFailureReason(syncCtx, &refuses, PlatformOpenAI, "", false, OpenAIEndpointCapabilityChatCompletions))
	require.False(t, isOpenAICompatibleAccountEligibleForRequest(syncCtx, &refuses, PlatformOpenAI, "", false, OpenAIEndpointCapabilityChatCompletions))
	require.True(t, isOpenAICompatibleAccountEligibleForRequest(syncCtx, &allows, PlatformOpenAI, "", false, OpenAIEndpointCapabilityChatCompletions))
	// 流式请求（未标记）不受开关影响。
	require.True(t, isOpenAICompatibleAccountEligibleForRequest(context.Background(), &refuses, PlatformOpenAI, "", false, OpenAIEndpointCapabilityChatCompletions))
	// alpha search 走同一判定。
	require.Equal(t, "sync_upstream_disabled",
		openAICompatibleAccountEligibilityFailureReason(syncCtx, &refuses, PlatformOpenAI, "", false, OpenAIEndpointCapabilityAlphaSearch))
}

func TestSelectAccountWithScheduler_SyncRequestSkipsAccountThatRefusesSync(t *testing.T) {
	for _, mode := range openAIImageSizeSchedulerModes {
		t.Run(mode.name, func(t *testing.T) {
			groupID := int64(94001)
			accounts := []Account{
				// 优先级更高（数值更小）的账号上游不接受同步请求。
				newOpenAISyncUpstreamTestAccount(82001, 0, groupID, true),
				newOpenAISyncUpstreamTestAccount(82002, 5, groupID, false),
			}
			svc := newOpenAIImageSizeSchedulerTestService(mode, accounts, nil, schedulerTestConcurrencyCache{})
			syncCtx := WithOpenAISyncUpstreamRequest(context.Background())

			// 重复多次：高级调度器带权重随机，单次命中不能说明该账号被排除。
			for i := 0; i < 20; i++ {
				selection, err := selectOpenAISyncUpstreamTestAccount(svc, syncCtx, groupID, "")
				require.NoError(t, err)
				require.Equal(t, int64(82002), selection.Account.ID, "同步请求必须换到接受同步的账号")
				if selection.ReleaseFunc != nil {
					selection.ReleaseFunc()
				}
			}

			// 流式请求不受开关影响：排除另一个账号后，该账号照常可选。
			streaming, err := selectOpenAISyncUpstreamTestAccount(svc, context.Background(), groupID, "", 82002)
			require.NoError(t, err)
			require.Equal(t, int64(82001), streaming.Account.ID)

			// 同步请求在只剩该账号时宁可失败，也不派给它。
			selection, err := selectOpenAISyncUpstreamTestAccount(svc, syncCtx, groupID, "", 82002)
			require.Error(t, err)
			require.True(t, selection == nil || selection.Account == nil)
		})
	}
}

func TestSelectAccountWithScheduler_SyncRequestFailsWhenEveryAccountRefusesSync(t *testing.T) {
	for _, mode := range openAIImageSizeSchedulerModes {
		t.Run(mode.name, func(t *testing.T) {
			groupID := int64(94002)
			accounts := []Account{
				newOpenAISyncUpstreamTestAccount(82011, 0, groupID, true),
				newOpenAISyncUpstreamTestAccount(82012, 5, groupID, true),
			}
			svc := newOpenAIImageSizeSchedulerTestService(mode, accounts, nil, schedulerTestConcurrencyCache{})

			selection, err := selectOpenAISyncUpstreamTestAccount(svc, WithOpenAISyncUpstreamRequest(context.Background()), groupID, "")
			require.Error(t, err)
			require.True(t, selection == nil || selection.Account == nil)

			streaming, err := selectOpenAISyncUpstreamTestAccount(svc, context.Background(), groupID, "")
			require.NoError(t, err)
			require.Contains(t, []int64{82011, 82012}, streaming.Account.ID)
		})
	}
}

// 会话已粘在不接受同步请求的账号上：同步请求换到能接的账号，会话随之改绑过去，
// 之后同一会话的流式与同步请求都落在新账号上，不会在两个账号之间来回切换。
func TestSelectAccountWithScheduler_SyncRequestRebindsStickySessionToAccountThatAcceptsSync(t *testing.T) {
	for _, mode := range openAIImageSizeSchedulerModes {
		t.Run(mode.name, func(t *testing.T) {
			groupID := int64(94003)
			sessionHash := "sync-upstream-sticky"
			accounts := []Account{
				newOpenAISyncUpstreamTestAccount(82021, 0, groupID, true),
				newOpenAISyncUpstreamTestAccount(82022, 5, groupID, false),
			}
			cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:" + sessionHash: 82021}}
			svc := newOpenAIImageSizeSchedulerTestService(mode, accounts, cache, schedulerTestConcurrencyCache{})

			// 流式请求照常命中粘性账号。
			streaming, err := selectOpenAISyncUpstreamTestAccount(svc, context.Background(), groupID, sessionHash)
			require.NoError(t, err)
			require.Equal(t, int64(82021), streaming.Account.ID)
			if streaming.ReleaseFunc != nil {
				streaming.ReleaseFunc()
			}

			sync, err := selectOpenAISyncUpstreamTestAccount(svc, WithOpenAISyncUpstreamRequest(context.Background()), groupID, sessionHash)
			require.NoError(t, err)
			require.Equal(t, int64(82022), sync.Account.ID)
			require.Equal(t, int64(82022), cache.sessionBindings["openai:"+sessionHash])
		})
	}
}
