package admin

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUpdateGatewayHeaderAuthSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"enabled":true,"rules":[{"header_name":"User-Agent","required_substring":"XundaAI"},{"header_name":"X-Client-Auth","required_substring":"allowed-app"}]}`, 200},
		{`{"enabled":false,"rules":[{"header_name":"x-app-token","required_substring":"client-id"}]}`, 200},
		{`{"enabled":true,"rules":[{"header_name":"User-Agent","required_substring":"XundaAI"},{"header_name":"Bad Header","required_substring":"x"}]}`, 400},
		{`{"enabled":true,"rules":[{"header_name":"User-Agent","required_substring":""}]}`, 400},
		{`{"enabled":true,"rules":[{"header_name":"User-Agent","required_substring":"bad\r\nvalue"}]}`, 400},
		{`{"enabled":true,"rules":[]}`, 400},
		{`{"enabled":true,"rules":null}`, 400},
		{`{"enabled":true,"rules":[null]}`, 400},
		{`{"enabled":false,"rules":[]}`, 200},
		{`{"enabled":true}`, 400},
		{`{"rules":[{"header_name":"User-Agent","required_substring":"XundaAI"}]}`, 400},
		// A stale single-rule UI must not silently replace new multi-rule policy.
		{`{"enabled":true,"header_name":"User-Agent","required_substring":"XundaAI"}`, 400},
	} {
		repo := &dailyIPSettingHandlerRepo{settingHandlerRepoStub: settingHandlerRepoStub{values: map[string]string{}}}
		h := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), nil, nil, nil, nil, nil, nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PUT", "/", strings.NewReader(tc.body))
		c.Request.Header.Set("Content-Type", "application/json")
		h.UpdateGatewayHeaderAuthSettings(c)
		require.Equal(t, tc.code, w.Code, tc.body)
		if tc.code == 200 {
			require.NotEmpty(t, repo.values[service.SettingKeyGatewayHeaderAuth])
		} else {
			require.Empty(t, repo.values[service.SettingKeyGatewayHeaderAuth])
		}
	}
}
