package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDailyIPTokenUsageIndexMigration(t *testing.T) {
	content, err := FS.ReadFile("242_daily_ip_token_usage_index_notx.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_ip_daily_tokens")
	require.Contains(t, sql, "ON usage_logs (ip_address, created_at)")
	require.Contains(t, sql, "INCLUDE (input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens)")
}

func TestDailyIPTokenQuotaSettingsMigrationPreservesExistingSettings(t *testing.T) {
	content, err := FS.ReadFile("243_daily_ip_token_quota_settings.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, `INSERT INTO settings (key, value, updated_at)`)
	require.Contains(t, sql, `"daily_token_limit":100000000`)
	require.Contains(t, sql, `"timezone":"Asia/Shanghai"`)
	require.Contains(t, sql, "ON CONFLICT (key) DO NOTHING")
}
