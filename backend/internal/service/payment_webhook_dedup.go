package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"entgo.io/ent/dialect"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

func paymentWebhookDedupKey(providerKey, eventID string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(providerKey)) + "\x00" + strings.TrimSpace(eventID)))
	return "wh:" + hex.EncodeToString(sum[:16])
}

// claimPaymentWebhookEvent persists a provider event_id once. A false claimed
// value means this event was already processed and the caller should ack it
// without fulfilling again.
func (s *PaymentService) claimPaymentWebhookEvent(ctx context.Context, providerKey, eventID, outTradeNo string) (bool, error) {
	if s == nil || s.entClient == nil {
		return true, nil
	}
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return true, nil
	}
	key := paymentWebhookDedupKey(providerKey, eventID)
	detail := fmt.Sprintf(`{"provider":%q,"event_id":%q,"out_trade_no":%q}`, strings.TrimSpace(providerKey), eventID, strings.TrimSpace(outTradeNo))
	query, args := buildPaymentWebhookDedupClaimQuery(s.entClient, key, detail)
	rows, err := s.entClient.QueryContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("claim webhook event: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	var claimID int64
	if err := rows.Scan(&claimID); err != nil {
		return false, err
	}
	return true, nil
}

func buildPaymentWebhookDedupClaimQuery(client *dbent.Client, orderID, detail string) (string, []any) {
	nowExpr := paymentAuditCurrentTimestampExpr(client)
	if paymentAuditDialect(client) == dialect.Postgres {
		return fmt.Sprintf(`
INSERT INTO payment_audit_logs (order_id, action, detail, operator, created_at)
VALUES ($1::text, 'WEBHOOK_DEDUP', $2::text, 'system', %s)
ON CONFLICT (order_id, action) DO NOTHING
RETURNING id`, nowExpr), []any{orderID, detail}
	}
	return fmt.Sprintf(`
INSERT INTO payment_audit_logs (order_id, action, detail, operator, created_at)
VALUES (?, 'WEBHOOK_DEDUP', ?, 'system', %s)
ON CONFLICT (order_id, action) DO NOTHING
RETURNING id`, nowExpr), []any{orderID, detail}
}
