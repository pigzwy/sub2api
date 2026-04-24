-- Restore payment base tables that later payment migrations depend on.
-- This file intentionally runs before 112_add_payment_order_provider_key_snapshot.sql.

CREATE TABLE IF NOT EXISTS payment_orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    user_email VARCHAR(255) NOT NULL DEFAULT '',
    user_name VARCHAR(100) NOT NULL DEFAULT '',
    user_notes TEXT,
    amount DECIMAL(20,2) NOT NULL,
    pay_amount DECIMAL(20,2) NOT NULL,
    fee_rate DECIMAL(10,4) NOT NULL DEFAULT 0,
    recharge_code VARCHAR(64) NOT NULL DEFAULT '',
    out_trade_no VARCHAR(64) NOT NULL DEFAULT '',
    payment_type VARCHAR(30) NOT NULL DEFAULT '',
    payment_trade_no VARCHAR(128) NOT NULL DEFAULT '',
    pay_url TEXT,
    qr_code TEXT,
    qr_code_img TEXT,
    order_type VARCHAR(20) NOT NULL DEFAULT 'balance',
    plan_id BIGINT,
    subscription_group_id BIGINT,
    subscription_days INT,
    provider_instance_id VARCHAR(64),
    provider_key VARCHAR(30),
    provider_snapshot JSONB,
    status VARCHAR(30) NOT NULL DEFAULT 'PENDING',
    refund_amount DECIMAL(20,2) NOT NULL DEFAULT 0,
    refund_reason TEXT,
    refund_at TIMESTAMPTZ,
    force_refund BOOLEAN NOT NULL DEFAULT FALSE,
    refund_requested_at TIMESTAMPTZ,
    refund_request_reason TEXT,
    refund_requested_by VARCHAR(20),
    expires_at TIMESTAMPTZ NOT NULL,
    paid_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    failed_reason TEXT,
    client_ip VARCHAR(50) NOT NULL DEFAULT '',
    src_host VARCHAR(255) NOT NULL DEFAULT '',
    src_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Also repair partially migrated databases where the base table exists but
-- follow-up payment columns were never applied.
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS out_trade_no VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS provider_key VARCHAR(30);
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS provider_snapshot JSONB;

CREATE INDEX IF NOT EXISTS idx_payment_orders_user_id ON payment_orders(user_id);
CREATE INDEX IF NOT EXISTS idx_payment_orders_status ON payment_orders(status);
CREATE INDEX IF NOT EXISTS idx_payment_orders_expires_at ON payment_orders(expires_at);
CREATE INDEX IF NOT EXISTS idx_payment_orders_created_at ON payment_orders(created_at);
CREATE INDEX IF NOT EXISTS idx_payment_orders_paid_at ON payment_orders(paid_at);
CREATE INDEX IF NOT EXISTS idx_payment_orders_type_paid ON payment_orders(payment_type, paid_at);
CREATE INDEX IF NOT EXISTS idx_payment_orders_order_type ON payment_orders(order_type);

CREATE TABLE IF NOT EXISTS payment_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    order_id VARCHAR(64) NOT NULL,
    action VARCHAR(50) NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    operator VARCHAR(100) NOT NULL DEFAULT 'system',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payment_audit_logs_order_id ON payment_audit_logs(order_id);

CREATE TABLE IF NOT EXISTS subscription_plans (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    price DECIMAL(20,2) NOT NULL,
    original_price DECIMAL(20,2),
    validity_days INT NOT NULL DEFAULT 30,
    validity_unit VARCHAR(10) NOT NULL DEFAULT 'day',
    features TEXT NOT NULL DEFAULT '',
    product_name VARCHAR(100) NOT NULL DEFAULT '',
    for_sale BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subscription_plans_group_id ON subscription_plans(group_id);
CREATE INDEX IF NOT EXISTS idx_subscription_plans_for_sale ON subscription_plans(for_sale);

CREATE TABLE IF NOT EXISTS payment_provider_instances (
    id BIGSERIAL PRIMARY KEY,
    provider_key VARCHAR(30) NOT NULL,
    name VARCHAR(100) NOT NULL DEFAULT '',
    config TEXT NOT NULL,
    supported_types VARCHAR(200) NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    payment_mode VARCHAR(20) NOT NULL DEFAULT '',
    sort_order INT NOT NULL DEFAULT 0,
    limits TEXT NOT NULL DEFAULT '',
    refund_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    allow_user_refund BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE payment_provider_instances ADD COLUMN IF NOT EXISTS payment_mode VARCHAR(20) NOT NULL DEFAULT '';
ALTER TABLE payment_provider_instances ADD COLUMN IF NOT EXISTS allow_user_refund BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE payment_provider_instances
SET payment_mode = 'redirect',
    supported_types = TRIM(BOTH ',' FROM REPLACE(REPLACE(REPLACE(
      supported_types, 'easypay,', ''), ',easypay', ''), 'easypay', ''))
WHERE provider_key = 'easypay' AND supported_types LIKE '%easypay%';

UPDATE payment_provider_instances
SET payment_mode = 'api'
WHERE provider_key = 'easypay' AND payment_mode = '';

CREATE INDEX IF NOT EXISTS idx_payment_provider_instances_provider_key ON payment_provider_instances(provider_key);
CREATE INDEX IF NOT EXISTS idx_payment_provider_instances_enabled ON payment_provider_instances(enabled);
