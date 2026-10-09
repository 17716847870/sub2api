package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dailyIPSettingHandlerRepo struct{ settingHandlerRepoStub }

func (r *dailyIPSettingHandlerRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func TestUpdateDailyIPTokenQuotaSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"enabled":true,"daily_token_limit":100000000,"timezone":"Asia/Shanghai"}`, 200},
		{`{"enabled":false,"daily_token_limit":200000000,"timezone":"UTC"}`, 200},
		{`{"enabled":true,"daily_token_limit":0,"timezone":"Asia/Shanghai"}`, 400},
		{`{"enabled":true,"daily_token_limit":-1,"timezone":"Asia/Shanghai"}`, 400},
		{`{"enabled":true,"daily_token_limit":1.5,"timezone":"Asia/Shanghai"}`, 400},
		{`{"enabled":true,"daily_token_limit":100,"timezone":"Bad/Timezone"}`, 400},
		{`{"enabled":false}`, 400},
	} {
		repo := &dailyIPSettingHandlerRepo{settingHandlerRepoStub: settingHandlerRepoStub{values: map[string]string{}}}
		h := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), nil, nil, nil, nil, nil, nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PUT", "/", strings.NewReader(tc.body))
		c.Request.Header.Set("Content-Type", "application/json")
		h.UpdateDailyIPTokenQuotaSettings(c)
		require.Equal(t, tc.code, w.Code, tc.body)
		if tc.code == http.StatusOK {
			require.NotEmpty(t, repo.values[service.SettingKeyDailyIPTokenQuota])
		} else {
			require.Empty(t, repo.values[service.SettingKeyDailyIPTokenQuota])
		}
	}
}

func TestDailyIPLimitedIPsRejectsInvalidPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, query := range []string{"?page=0", "?page=no", "?page_size=101", "?page_size=-1", "?search=" + strings.Repeat("1", 46)} {
		h := &SettingHandler{}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/"+query, nil)
		h.ListDailyIPLimitedIPs(c)
		require.Equal(t, 400, w.Code, query)
	}
}
