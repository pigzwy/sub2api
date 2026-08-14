CREATE TABLE IF NOT EXISTS user_checkin_records (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    checkin_date DATE NOT NULL,
    amount DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One check-in per user per day. This is the enforcement point, not just an
-- index: the insert races with itself when a user double-submits, and the
-- unique violation is what turns the second attempt into "already signed"
-- rather than a second reward.
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_checkin_records_user_date
    ON user_checkin_records (user_id, checkin_date);

-- Serves the monthly calendar and the recent-records list, both of which read
-- one user's rows in date order.
CREATE INDEX IF NOT EXISTS idx_user_checkin_records_user_date_desc
    ON user_checkin_records (user_id, checkin_date DESC);

COMMENT ON TABLE user_checkin_records IS
    'Daily check-in rewards; one row per user per day, written in the same transaction as the balance credit';
COMMENT ON COLUMN user_checkin_records.checkin_date IS
    'Calendar day in the server timezone, supplied by the application rather than CURRENT_DATE so it matches what the user sees';
COMMENT ON COLUMN user_checkin_records.amount IS
    'Balance credited for this check-in, in the same unit as users.balance';
