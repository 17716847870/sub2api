package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gatewayHeaderRouteSettingsRepo struct {
	service.SettingRepository
	value string
}

func (r *gatewayHeaderRouteSettingsRepo) GetValue(context.Context, string) (string, error) {
	if r.value == "" {
		return "", service.ErrSettingNotFound
	}
	return r.value, nil
}
func (r *gatewayHeaderRouteSettingsRepo) Set(_ context.Context, _, value string) error {
	r.value = value
	return nil
}

func gatewayHeaderAuthTestRouter(t *testing.T) (*gin.Engine, *service.SettingService, *int) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	settings := service.NewSettingService(&gatewayHeaderRouteSettingsRepo{}, &config.Config{})
	calls := 0
	auth := middleware.APIKeyAuthMiddleware(func(c *gin.Context) { calls++; c.AbortWithStatus(http.StatusAccepted) })
	RegisterGatewayRoutes(r, &handler.Handlers{Gateway: &handler.GatewayHandler{}, OpenAIGateway: &handler.OpenAIGatewayHandler{}, AsyncImage: handler.NewAsyncImageHandler(nil, nil)}, auth, nil, nil, nil, settings, nil, &config.Config{Gateway: config.GatewayConfig{MaxBodySize: 1024 * 1024, TextMaxBodySize: 1024 * 1024}}, nil)
	return r, settings, &calls
}

func TestGatewayRoutesHeaderAuthenticationCoversEveryEntry(t *testing.T) {
	r, _, calls := gatewayHeaderAuthTestRouter(t)
	parameters := regexp.MustCompile(`:[^/]+`)
	for _, route := range r.Routes() {
		path := parameters.ReplaceAllString(route.Path, "test")
		path = strings.ReplaceAll(path, "*subpath", "compact")
		path = strings.ReplaceAll(path, "*modelAction", "gemini:generateContent")
		t.Run(route.Method+" "+path, func(t *testing.T) {
			request := httptest.NewRequest(route.Method, path, strings.NewReader(`{"model":"test"}`))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, request)
			require.Equal(t, 401, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), "Request header authentication failed.")
		})
	}
	require.Zero(t, *calls, "missing header must be rejected before API key lookup or upstream forwarding")
}

func TestGatewayRoutesHeaderAuthenticationStillRequiresAPIKeyAndCanDisable(t *testing.T) {
	r, settings, calls := gatewayHeaderAuthTestRouter(t)
	for _, path := range []string{"/v1/messages", "/responses", "/backend-api/codex/responses", "/antigravity/models"} {
		method := http.MethodPost
		if path == "/antigravity/models" {
			method = http.MethodGet
		}
		request := httptest.NewRequest(method, path, nil)
		request.Header.Set("User-Agent", "other-client XundaAI/1.0")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, request)
		require.Equal(t, 202, w.Code, "matching header must continue into API key authentication")
	}
	require.Equal(t, 4, *calls)
	updated := service.DefaultGatewayHeaderAuthSettings()
	updated.Enabled = false
	require.NoError(t, settings.SetGatewayHeaderAuthSettings(context.Background(), updated))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/messages", nil))
	require.Equal(t, 202, w.Code)
	require.Equal(t, 5, *calls)
}

func TestGatewayRoutesHeaderAuthenticationRejectsWebSocketHandshake(t *testing.T) {
	r, _, calls := gatewayHeaderAuthTestRouter(t)
	for _, path := range []string{"/responses", "/v1/responses", "/backend-api/codex/responses", "/v1/realtime"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Upgrade", "websocket")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, request)
		require.Equal(t, 401, w.Code)
		require.Contains(t, w.Body.String(), middleware.GatewayHeaderAuthFailedCode)
	}
	require.Zero(t, *calls)
}

func TestGatewayRoutesHeaderAuthenticationMultipleRules(t *testing.T) {
	router, settings, calls := gatewayHeaderAuthTestRouter(t)
	policy := service.DefaultGatewayHeaderAuthSettings()
	policy.Rules = append(policy.Rules, service.GatewayHeaderAuthRule{HeaderName: "X-Client-Auth", RequiredSubstring: "allowed-app"})
	require.NoError(t, settings.SetGatewayHeaderAuthSettings(context.Background(), policy))
	for _, path := range []string{"/v1/messages", "/responses", "/backend-api/codex/responses"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("User-Agent", "XundaAI/1.0")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, 401, w.Code)
		req.Header.Set("X-Client-Auth", "allowed-app/v2")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, 202, w.Code)
	}
	require.Equal(t, 3, *calls, "API key lookup must happen only when all headers match")
}
