-- 228_usage_log_audio_tokens.sql
-- usage_logs 单独记录 Realtime 语音的 audio token 数与费用，便于语音会话对账。
-- audio_input_tokens 从 input_tokens 中拆出（非缓存音频），audio_output_tokens 从
-- output_tokens 中拆出；对应费用从 input_cost/output_cost 中拆出，total_cost 口径不变。
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS audio_input_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS audio_input_cost DECIMAL(20, 10) NOT NULL DEFAULT 0;
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS audio_output_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS audio_output_cost DECIMAL(20, 10) NOT NULL DEFAULT 0;
