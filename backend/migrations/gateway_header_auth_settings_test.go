package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayHeaderAuthSettingsMigrationPreservesExistingValues(t *testing.T) {
	content, err := FS.ReadFile("244_gateway_header_auth_settings.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, `"header_name":"User-Agent"`)
	require.Contains(t, sql, `"required_substring":"XundaAI"`)
	require.Contains(t, sql, `"enabled":true`)
	require.Contains(t, sql, "ON CONFLICT (key) DO NOTHING")
}
