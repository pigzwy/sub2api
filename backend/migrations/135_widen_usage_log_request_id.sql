-- Allow provider/request identifiers longer than the original 64-byte limit.
-- Recent upstream usage logging may persist raw upstream request ids, which can
-- exceed VARCHAR(64). Keep the existing idempotency index semantics intact.

ALTER TABLE usage_logs
    ALTER COLUMN request_id TYPE TEXT;
