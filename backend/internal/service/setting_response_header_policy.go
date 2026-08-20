package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// 读取频率是每个请求一次，因此沿用本仓其它运行时设置的做法：进程内 atomic 缓存
// + singleflight，热路径零锁。
const (
	responseHeaderPolicyCacheTTL = 60 * time.Second
	// 读库失败时短缓存快速重试，同时保持默认（隐藏）——失败不该导致泄露。
	responseHeaderPolicyErrorTTL  = 5 * time.Second
	responseHeaderPolicyDBTimeout = 5 * time.Second
)

// ResponseHeaderPolicy 控制上游响应头能否到达客户端。
type ResponseHeaderPolicy struct {
	// HideUpstream 剥掉会暴露上游身份与配额的响应头：x-request-id、x-ratelimit-*、
	// x-codex-*。上游本身是中转（sub2api / new-api 等）时，这些值属于中转站，透传
	// 出去既暴露转发架构，也会让照着限额排查的人看错数。
	HideUpstream bool `json:"hide_upstream"`
}

// DefaultResponseHeaderPolicy 默认隐藏：多数部署的上游是中转或聚合站。
func DefaultResponseHeaderPolicy() *ResponseHeaderPolicy {
	return &ResponseHeaderPolicy{HideUpstream: true}
}

type cachedResponseHeaderPolicy struct {
	hideUpstream bool
	expiresAt    int64 // unix nano
}

var (
	responseHeaderPolicyCache atomic.Value // *cachedResponseHeaderPolicy
	responseHeaderPolicySF    singleflight.Group
)

// GetResponseHeaderPolicy 读取响应头策略；未配置过时返回默认值。
func (s *SettingService) GetResponseHeaderPolicy(ctx context.Context) (*ResponseHeaderPolicy, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyResponseHeaderPolicy)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultResponseHeaderPolicy(), nil
		}
		return nil, fmt.Errorf("get response header policy: %w", err)
	}
	if value == "" {
		return DefaultResponseHeaderPolicy(), nil
	}
	var policy ResponseHeaderPolicy
	if err := json.Unmarshal([]byte(value), &policy); err != nil {
		// 解析不了就按默认走，宁可多隐藏也不要因为一条坏数据开始泄露。
		return DefaultResponseHeaderPolicy(), nil
	}
	return &policy, nil
}

// SetResponseHeaderPolicy 保存策略并让本副本立即生效。
func (s *SettingService) SetResponseHeaderPolicy(ctx context.Context, policy *ResponseHeaderPolicy) error {
	if policy == nil {
		return fmt.Errorf("response header policy cannot be nil")
	}
	data, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("marshal response header policy: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyResponseHeaderPolicy, string(data)); err != nil {
		return err
	}
	// 本副本立即生效；其余副本靠中间件的缓存过期后自行收敛。
	InvalidateResponseHeaderPolicyCache()
	return nil
}

// HidesUpstreamResponseHeaders 供中间件每请求调用，带 60 秒缓存。
func (s *SettingService) HidesUpstreamResponseHeaders(ctx context.Context) bool {
	if s == nil {
		return true
	}
	if cached, ok := responseHeaderPolicyCache.Load().(*cachedResponseHeaderPolicy); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.hideUpstream
		}
	}
	val, _, _ := responseHeaderPolicySF.Do(SettingKeyResponseHeaderPolicy, func() (any, error) {
		if cached, ok := responseHeaderPolicyCache.Load().(*cachedResponseHeaderPolicy); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.hideUpstream, nil
			}
		}
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), responseHeaderPolicyDBTimeout)
		defer cancel()
		policy, err := s.GetResponseHeaderPolicy(dbCtx)
		if err != nil {
			slog.Warn("failed to read response header policy", "error", err)
			responseHeaderPolicyCache.Store(&cachedResponseHeaderPolicy{
				hideUpstream: true,
				expiresAt:    time.Now().Add(responseHeaderPolicyErrorTTL).UnixNano(),
			})
			return true, nil
		}
		responseHeaderPolicyCache.Store(&cachedResponseHeaderPolicy{
			hideUpstream: policy.HideUpstream,
			expiresAt:    time.Now().Add(responseHeaderPolicyCacheTTL).UnixNano(),
		})
		return policy.HideUpstream, nil
	})
	if v, ok := val.(bool); ok {
		return v
	}
	return true
}

// InvalidateResponseHeaderPolicyCache 让保存后立即生效，无需等缓存过期。
func InvalidateResponseHeaderPolicyCache() {
	responseHeaderPolicyCache.Store(&cachedResponseHeaderPolicy{})
}
