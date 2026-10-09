package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gatewayDailyIPUsageStub struct{}

func (gatewayDailyIPUsageStub) GetTokenUsageByIP(context.Context, string, time.Time, time.Time) (int64, error) {
	return service.DefaultDailyIPTokenLimit, nil
}

func TestGatewayRoutesDailyIPTokenQuotaBlocksAliasesAndStreams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxBodySize: 1024 * 1024, TextMaxBodySize: 1024 * 1024}}
	quota, err := service.NewDailyIPTokenQuotaService(gatewayDailyIPUsageStub{}, nil)
	require.NoError(t, err)
	auth := servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{})
		c.Next()
	})
	RegisterGatewayRoutes(router, &handler.Handlers{
		Gateway: &handler.GatewayHandler{}, OpenAIGateway: &handler.OpenAIGatewayHandler{}, AsyncImage: handler.NewAsyncImageHandler(nil, nil),
	}, auth, nil, nil, nil, nil, nil, cfg, gin.HandlerFunc(servermiddleware.NewDailyIPTokenQuotaMiddleware(quota)))
	for _, path := range []string{
		"/v1/messages", "/v1/chat/completions", "/chat/completions", "/v1/responses", "/responses", "/responses/compact",
		"/backend-api/codex/responses", "/backend-api/codex/responses/compact", "/antigravity/v1/messages",
		"/v1/images/generations", "/images/generations", "/v1/images/generations/async", "/v1/images/batches", "/v1/embeddings",
	} {
		t.Run(path, func(t *testing.T) {
			r := httptest.NewRequest("POST", path, strings.NewReader(`{"model":"test","stream":true}`))
			r.RemoteAddr = "203.0.113.8:1234"
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			require.Equal(t, http.StatusTooManyRequests, w.Code, "%s: %s", path, w.Body.String())
			require.Contains(t, w.Body.String(), "daily_ip_token_limit_exceeded")
		})
	}
	for _, path := range []string{"/v1/responses", "/responses", "/backend-api/codex/responses"} {
		r := httptest.NewRequest("GET", path, nil)
		r.RemoteAddr = "203.0.113.8:1234"
		r.Header.Set("Upgrade", "websocket")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		require.Equal(t, http.StatusTooManyRequests, w.Code)
	}
}

func TestGatewayRoutesDailyIPTokenQuotaMountedAfterAllAuthPaths(t *testing.T) {
	content, err := os.ReadFile("gateway.go")
	require.NoError(t, err)
	source := string(content)
	for _, group := range []string{"gateway", "gemini", "antigravityV1", "antigravityV1Beta"} {
		marker := group + ".Use(quota)"
		require.Equal(t, 1, strings.Count(source, marker))
		quotaAt := strings.Index(source, marker)
		authAt := strings.LastIndex(source[:quotaAt], group+".Use(")
		require.Contains(t, strings.ToLower(source[authAt:quotaAt]), "apikeyauth", "quota must follow successful authentication: %s", group)
	}
	require.Contains(t, source, "gin.HandlerFunc(apiKeyAuth), quota, groupModelAllowlist")
}
