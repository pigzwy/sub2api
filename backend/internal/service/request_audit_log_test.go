package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

func TestShouldCaptureRequestAuditScopeSemantics(t *testing.T) {
	groupID := int64(20)
	otherGroupID := int64(30)

	tests := []struct {
		name          string
		userID        int64
		groupID       *int64
		scopeUserIDs  []int64
		scopeGroupIDs []int64
		want          bool
	}{
		{
			name:    "empty scopes audit all requests",
			userID:  10,
			groupID: &groupID,
			want:    true,
		},
		{
			name:         "only users audits matching user across all groups",
			userID:       10,
			groupID:      &groupID,
			scopeUserIDs: []int64{10},
			want:         true,
		},
		{
			name:         "only users skips non matching user",
			userID:       11,
			groupID:      &groupID,
			scopeUserIDs: []int64{10},
			want:         false,
		},
		{
			name:          "only groups audits all users in matching group",
			userID:        11,
			groupID:       &groupID,
			scopeGroupIDs: []int64{20},
			want:          true,
		},
		{
			name:          "only groups skips non matching group",
			userID:        11,
			groupID:       &otherGroupID,
			scopeGroupIDs: []int64{20},
			want:          false,
		},
		{
			name:          "users and groups require intersection",
			userID:        10,
			groupID:       &groupID,
			scopeUserIDs:  []int64{10},
			scopeGroupIDs: []int64{20},
			want:          true,
		},
		{
			name:          "users and groups skip matching user outside group",
			userID:        10,
			groupID:       &otherGroupID,
			scopeUserIDs:  []int64{10},
			scopeGroupIDs: []int64{20},
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldCaptureRequestAudit(tt.userID, tt.groupID, tt.scopeUserIDs, tt.scopeGroupIDs)
			if got != tt.want {
				t.Fatalf("ShouldCaptureRequestAudit() = %v, want %v", got, tt.want)
			}
		})
	}
}

type requestAuditMemoryRepo struct {
	created *RequestAuditLog
}

func (r *requestAuditMemoryRepo) Create(_ context.Context, log *RequestAuditLog) error {
	clone := *log
	r.created = &clone
	return nil
}

func (r *requestAuditMemoryRepo) List(_ context.Context, _ pagination.PaginationParams, _ RequestAuditLogFilter) ([]RequestAuditLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *requestAuditMemoryRepo) GetByID(_ context.Context, _ int64) (*RequestAuditLog, error) {
	return nil, ErrUsageLogNotFound
}

func (r *requestAuditMemoryRepo) Cleanup(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func TestRequestAuditLogServiceCreatePersistsMockFields(t *testing.T) {
	repo := &requestAuditMemoryRepo{}
	svc := NewRequestAuditLogService(repo)
	ruleID := int64(42)

	err := svc.Create(context.Background(), RequestAuditLogCreateInput{
		UserID:      1,
		APIKeyID:    2,
		Platform:    "openai",
		RequestBody: []byte(`{"messages":[]}`),
		IsMocked:    true,
		MockRuleID:  &ruleID,
	})

	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if repo.created == nil {
		t.Fatal("expected audit log to be created")
	}
	if !repo.created.IsMocked {
		t.Fatal("expected IsMocked to be true")
	}
	if repo.created.MockRuleID == nil || *repo.created.MockRuleID != ruleID {
		t.Fatalf("MockRuleID = %v, want %d", repo.created.MockRuleID, ruleID)
	}
}
