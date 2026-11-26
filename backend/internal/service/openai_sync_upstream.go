package service

import "context"

// AccountExtraOpenAISyncUpstreamDisabled marks an OpenAI-compatible account
// whose upstream refuses non-streaming requests. Some relay upstreams answer a
// synchronous /v1/responses or /v1/alpha/search call with a bare HTTP 400
// ("请求被拒绝", or code "stream_required") while serving the same request
// fine when it is streamed. The error is indistinguishable from a genuinely
// invalid request, so the gateway cannot fail over on it; the admin declares it
// on the account instead.
//
// Stored inverted (true = disabled) so that accounts without the key keep the
// existing behaviour of accepting synchronous requests.
const AccountExtraOpenAISyncUpstreamDisabled = "openai_sync_upstream_disabled"

// openAISyncUpstreamDisabledReason names the veto in scheduler filter stats and
// no-account diagnostics.
const openAISyncUpstreamDisabledReason = "sync_upstream_disabled"

// AllowsOpenAISyncUpstream reports whether synchronous (non-streaming) upstream
// requests may be scheduled onto this account. Defaults to true.
func (a *Account) AllowsOpenAISyncUpstream() bool {
	if a == nil {
		return false
	}
	return a.Extra[AccountExtraOpenAISyncUpstreamDisabled] != true
}

type openAISyncUpstreamCtxKey struct{}

// WithOpenAISyncUpstreamRequest marks a request that the gateway will send to
// the upstream without streaming, so account selection skips accounts that
// refuse such requests.
//
// Only mark requests whose upstream call really is synchronous. Synchronous
// /v1/chat/completions and /v1/messages calls are served by a streamed upstream
// request that the gateway buffers, so they must not be marked: that would
// needlessly route them away from accounts that handle them correctly.
func WithOpenAISyncUpstreamRequest(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAISyncUpstreamCtxKey{}, true)
}

func isOpenAISyncUpstreamRequest(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	marked, _ := ctx.Value(openAISyncUpstreamCtxKey{}).(bool)
	return marked
}

// openAISyncUpstreamVetoReason excludes accounts that refuse synchronous
// upstream requests from the selection of a request marked as synchronous.
// Streaming requests and unmarked requests are never vetoed.
func openAISyncUpstreamVetoReason(ctx context.Context, account *Account) (bool, string) {
	if account == nil || !isOpenAISyncUpstreamRequest(ctx) {
		return false, ""
	}
	if account.AllowsOpenAISyncUpstream() {
		return false, ""
	}
	return true, openAISyncUpstreamDisabledReason
}
