package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dailyIPQuotaRepoStub struct {
	used     int64
	err      error
	calls    int
	clientIP string
}

func (r *dailyIPQuotaRepoStub) GetTokenUsageByIP(_ context.Context, clientIP string, _, _ time.Time) (int64, error) {
	r.calls++
	r.clientIP = clientIP
	return r.used, r.err
}

func quotaTestRouter(t *testing.T, repo *dailyIPQuotaRepoStub, trusted ...string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(trusted))
	svc, err := service.NewDailyIPTokenQuotaService(repo, nil)
	require.NoError(t, err)
	router.Use(gin.HandlerFunc(NewDailyIPTokenQuotaMiddleware(svc)))
	router.Any("/*path", func(c *gin.Context) { c.String(http.StatusOK, ip.GetClientIP(c)) })
	return router
}

func quotaTestRequest(router *gin.Engine, method, path string, ws bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.RemoteAddr = "203.0.113.8:1234"
	if ws {
		r.Header.Set("Upgrade", "websocket")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestDailyIPTokenQuotaMiddlewareExhausted(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/v1/chat/completions", "/responses", "/v1beta/models/gemini:generateContent", "/antigravity/v1beta/models/gemini:streamGenerateContent"} {
		t.Run(path, func(t *testing.T) {
			repo := &dailyIPQuotaRepoStub{used: service.DefaultDailyIPTokenLimit}
			w := quotaTestRequest(quotaTestRouter(t, repo), http.MethodPost, path, false)
			require.Equal(t, http.StatusTooManyRequests, w.Code)
			require.NotEmpty(t, w.Header().Get("Retry-After"))
			require.Equal(t, "100000000", w.Header().Get("X-Daily-Token-Limit"))
			require.Equal(t, "100000000", w.Header().Get("X-Daily-Token-Used"))
			require.NotEmpty(t, w.Header().Get("X-Daily-Token-Reset"))
			if path == "/v1beta/models/gemini:generateContent" {
				require.Contains(t, w.Body.String(), "RESOURCE_EXHAUSTED")
			} else if path == "/v1/messages" {
				require.Contains(t, w.Body.String(), "daily_ip_token_limit_exceeded")
			}
		})
	}
}

func TestDailyIPTokenQuotaMiddlewareAllowedAndFailClosed(t *testing.T) {
	repo := &dailyIPQuotaRepoStub{used: service.DefaultDailyIPTokenLimit - 1}
	router := quotaTestRouter(t, repo)
	require.Equal(t, 200, quotaTestRequest(router, "POST", "/v1/messages", false).Code)
	repo.err = errors.New("database is down")
	w := quotaTestRequest(router, "POST", "/v1/messages", false)
	require.Equal(t, 503, w.Code)
	require.Contains(t, w.Body.String(), "daily_ip_token_quota_unavailable")
	require.NotContains(t, w.Body.String(), "database is down")
}

func TestDailyIPTokenQuotaMiddlewareReadOnlyAndWebSocket(t *testing.T) {
	repo := &dailyIPQuotaRepoStub{used: service.DefaultDailyIPTokenLimit}
	router := quotaTestRouter(t, repo)
	for _, path := range []string{"/v1/models", "/v1/usage", "/v1/sub2api/billing", "/v1/images/tasks/task-1"} {
		require.Equal(t, 200, quotaTestRequest(router, "GET", path, false).Code)
	}
	require.Equal(t, 200, quotaTestRequest(router, "POST", "/messages/count_tokens", false).Code)
	require.Equal(t, 200, quotaTestRequest(router, "POST", "/v1/images/batches/1/cancel", false).Code)
	require.Zero(t, repo.calls)
	require.Equal(t, 429, quotaTestRequest(router, "GET", "/v1/responses", true).Code)
	require.Equal(t, 1, repo.calls)
}

func TestDailyIPTokenQuotaMiddlewareTrustedIP(t *testing.T) {
	for _, trusted := range []bool{false, true} {
		repo := &dailyIPQuotaRepoStub{}
		var proxies []string
		if trusted {
			proxies = []string{"127.0.0.1"}
		}
		router := quotaTestRouter(t, repo, proxies...)
		r := httptest.NewRequest("POST", "/v1/messages", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("X-Forwarded-For", "203.0.113.8")
		r.Header.Set("CF-Connecting-IP", "198.51.100.99")
		r.Header.Set("X-Real-IP", "198.51.100.99")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		require.Equal(t, 200, w.Code)
		want := "127.0.0.1"
		if trusted {
			want = "203.0.113.8"
		}
		require.Equal(t, want, repo.clientIP)
		require.Equal(t, want, w.Body.String(), "admission and recorded usage must have the same address")
	}
}

func TestDailyIPTokenQuotaMiddlewareMappedIPv4(t *testing.T) {
	repo := &dailyIPQuotaRepoStub{}
	router := quotaTestRouter(t, repo)
	r := httptest.NewRequest("POST", "/v1/messages", nil)
	r.RemoteAddr = "[::ffff:203.0.113.8]:1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	require.Equal(t, 200, w.Code)
	require.Equal(t, "203.0.113.8", repo.clientIP)
	require.Equal(t, repo.clientIP, w.Body.String())
}
