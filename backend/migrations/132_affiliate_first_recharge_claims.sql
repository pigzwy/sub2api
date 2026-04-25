CREATE TABLE IF NOT EXISTS user_affiliate_first_recharge_claims (
    invitee_user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    inviter_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_ref VARCHAR(160) NOT NULL UNIQUE,
    amount DECIMAL(20,8) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_affiliate_first_recharge_claims_inviter_id
ON user_affiliate_first_recharge_claims(inviter_user_id);

COMMENT ON TABLE user_affiliate_first_recharge_claims IS '邀请返利首次充值领取幂等表';
COMMENT ON COLUMN user_affiliate_first_recharge_claims.invitee_user_id IS '被邀请人用户ID，每个被邀请人只能领取一次首次充值返利';
COMMENT ON COLUMN user_affiliate_first_recharge_claims.inviter_user_id IS '邀请人用户ID';
COMMENT ON COLUMN user_affiliate_first_recharge_claims.source_ref IS '首次充值来源引用，如 redeem:<code> 或 payment_order:<id>';

-- Existing affiliate accrual ledgers mean those invitees have already received
-- a recharge rebate before this first-recharge-only guard was introduced.
INSERT INTO user_affiliate_first_recharge_claims (
    invitee_user_id,
    inviter_user_id,
    source_ref,
    amount,
    created_at,
    updated_at
)
SELECT DISTINCT ON (source_user_id)
       source_user_id,
       user_id,
       'legacy-affiliate-ledger:' || id::text,
       amount,
       created_at,
       updated_at
FROM user_affiliate_ledger
WHERE action = 'accrue'
  AND source_user_id IS NOT NULL
ORDER BY source_user_id, id;
