-- Daily IP quotas filter by one IP and a half-open local-day timestamp range.
-- Concurrent creation avoids blocking writes to usage_logs during upgrades.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_ip_daily_tokens
    ON usage_logs (ip_address, created_at)
    INCLUDE (input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens);
