package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gatewayHeaderSettingsRepo struct {
	service.SettingRepository
	value string
	err   error
}

func (r *gatewayHeaderSettingsRepo) GetValue(context.Context, string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	if r.value == "" {
		return "", service.ErrSettingNotFound
	}
	return r.value, nil
}
func (r *gatewayHeaderSettingsRepo) Set(_ context.Context, _, value string) error {
	r.value = value
	return nil
}

func headerAuthRouter(t *testing.T, settings *service.SettingService, normalizeUA bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if normalizeUA {
		router.Use(SessionBindingContext(&config.Config{}))
	}
	router.Use(GatewayHeaderAuthentication(settings))
	router.Any("/*path", func(c *gin.Context) { c.String(http.StatusOK, "allowed") })
	return router
}
func headerAuthRequest(router *gin.Engine, path string, headers http.Header) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"User-Agent":"XundaAI"}`))
	r.Header = headers
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestGatewayHeaderAuthenticationMatching(t *testing.T) {
	for _, tc := range []struct {
		name, agent  string
		extraHeaders http.Header
		allowed      bool
	}{
		{name: "missing", agent: ""},
		{name: "wrong client", agent: "Mozilla/5.0"},
		{name: "wrong casing", agent: "xundaai/1.0"},
		{name: "exact match", agent: "XundaAI", allowed: true},
		{name: "substring", agent: "Mozilla/5.0 XundaAI/1.0 other", allowed: true},
		{name: "marker in another header", agent: "curl/1.0", extraHeaders: http.Header{"X-User-Agent": []string{"XundaAI"}}},
		{name: "multiple header values", extraHeaders: http.Header{"User-Agent": []string{"other", "XundaAI/1.0"}}, allowed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := service.NewSettingService(&gatewayHeaderSettingsRepo{}, &config.Config{})
			headers := tc.extraHeaders
			if headers == nil {
				headers = http.Header{}
			}
			if tc.agent != "" {
				headers.Set("User-Agent", tc.agent)
			}
			w := headerAuthRequest(headerAuthRouter(t, svc, false), "/v1/chat/completions?User-Agent=XundaAI", headers)
			if tc.allowed {
				require.Equal(t, 200, w.Code)
			} else {
				require.Equal(t, 401, w.Code)
				require.Contains(t, w.Body.String(), GatewayHeaderAuthFailedCode)
				require.Contains(t, w.Body.String(), "authentication_error")
				require.NotContains(t, w.Body.String(), "XundaAI", "error must not echo authentication requirements")
			}
		})
	}
}

func TestGatewayHeaderAuthenticationCustomSettingsAndHotUpdate(t *testing.T) {
	repo := &gatewayHeaderSettingsRepo{}
	svc := service.NewSettingService(repo, &config.Config{})
	router := headerAuthRouter(t, svc, false)
	settings := service.GatewayHeaderAuthSettings{Enabled: true, Rules: []service.GatewayHeaderAuthRule{{HeaderName: "x-client-auth", RequiredSubstring: "allowed-app"}}}
	require.NoError(t, svc.SetGatewayHeaderAuthSettings(context.Background(), settings))
	w := headerAuthRequest(router, "/v1/responses", http.Header{"X-Client-Auth": []string{"allowed-app/v2"}})
	require.Equal(t, 200, w.Code)
	w = headerAuthRequest(router, "/v1/responses", http.Header{"User-Agent": []string{"XundaAI"}})
	require.Equal(t, 401, w.Code)
	settings.Enabled = false
	require.NoError(t, svc.SetGatewayHeaderAuthSettings(context.Background(), settings))
	w = headerAuthRequest(router, "/v1/responses", http.Header{})
	require.Equal(t, 200, w.Code)
}

func TestGatewayHeaderAuthenticationErrorProtocolsAndFailClosed(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/messages", "/v1beta/models/gemini:generateContent", "/antigravity/v1beta/models/gemini:generateContent"} {
		w := headerAuthRequest(headerAuthRouter(t, service.NewSettingService(&gatewayHeaderSettingsRepo{}, &config.Config{}), false), path, http.Header{})
		require.Equal(t, 401, w.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		if strings.Contains(path, "v1beta") {
			require.Contains(t, w.Body.String(), "UNAUTHENTICATED")
		} else {
			require.Equal(t, "error", body["type"])
		}
	}
	w := headerAuthRequest(headerAuthRouter(t, service.NewSettingService(&gatewayHeaderSettingsRepo{err: errors.New("database password secret")}, &config.Config{}), false), "/v1/responses", http.Header{"User-Agent": []string{"XundaAI"}})
	require.Equal(t, 503, w.Code)
	require.Equal(t, "5", w.Header().Get("Retry-After"))
	require.NotContains(t, w.Body.String(), "database password secret")
}

func TestGatewayHeaderAuthenticationUsesOriginalUserAgent(t *testing.T) {
	// Metadata normalization truncates UA to 512 bytes, but admission must inspect
	// the actual received header (including later substrings and repeated fields).
	ua := strings.Repeat("a", maxPersistentUserAgentBytes+20) + " XundaAI/1.0"
	router := headerAuthRouter(t, service.NewSettingService(&gatewayHeaderSettingsRepo{}, &config.Config{}), true)
	w := headerAuthRequest(router, "/v1/messages", http.Header{"User-Agent": []string{ua}})
	require.Equal(t, 200, w.Code)
	w = headerAuthRequest(router, "/v1/messages", http.Header{"User-Agent": []string{"other", "XundaAI/1.0"}})
	require.Equal(t, 200, w.Code)
	w = headerAuthRequest(router, "/v1/messages", http.Header{})
	require.Equal(t, 401, w.Code)
}

func TestGatewayHeaderAuthenticationAllRulesRequired(t *testing.T) {
	svc := service.NewSettingService(&gatewayHeaderSettingsRepo{}, &config.Config{})
	settings := service.GatewayHeaderAuthSettings{Enabled: true, Rules: []service.GatewayHeaderAuthRule{
		{HeaderName: "User-Agent", RequiredSubstring: "XundaAI"},
		{HeaderName: "X-Client-Auth", RequiredSubstring: "allowed-app"},
	}}
	require.NoError(t, svc.SetGatewayHeaderAuthSettings(context.Background(), settings))
	router := headerAuthRouter(t, svc, true)
	for _, tc := range []struct {
		name    string
		headers http.Header
		code    int
	}{
		{"no headers", http.Header{}, 401},
		{"only first matches", http.Header{"User-Agent": {"XundaAI/1.0"}}, 401},
		{"only second matches", http.Header{"X-Client-Auth": {"allowed-app"}}, 401},
		{"second wrong", http.Header{"User-Agent": {"XundaAI"}, "X-Client-Auth": {"other"}}, 401},
		{"second wrong casing", http.Header{"User-Agent": {"XundaAI"}, "X-Client-Auth": {"Allowed-App"}}, 401},
		{"all match", http.Header{"User-Agent": {"XundaAI/1.0"}, "X-Client-Auth": {"prefix allowed-app/v2"}}, 200},
		{"multiple values in each header", http.Header{"User-Agent": {"other", "XundaAI"}, "X-Client-Auth": {"other", "allowed-app"}}, 200},
		{"marker split across values", http.Header{"User-Agent": {"XundaAI"}, "X-Client-Auth": {"allowed-", "app"}}, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := headerAuthRequest(router, "/v1/messages", tc.headers)
			require.Equal(t, tc.code, w.Code)
			if tc.code == 401 {
				require.Contains(t, w.Body.String(), GatewayHeaderAuthFailedCode)
				require.NotContains(t, w.Body.String(), "allowed-app")
			}
		})
	}
	// Two requirements for the same header must also both match.
	settings.Rules = []service.GatewayHeaderAuthRule{{HeaderName: "User-Agent", RequiredSubstring: "XundaAI"}, {HeaderName: "User-Agent", RequiredSubstring: "/v2"}}
	require.NoError(t, svc.SetGatewayHeaderAuthSettings(context.Background(), settings))
	require.Equal(t, 401, headerAuthRequest(router, "/v1/messages", http.Header{"User-Agent": {"XundaAI/v1"}}).Code)
	require.Equal(t, 200, headerAuthRequest(router, "/v1/messages", http.Header{"User-Agent": {"XundaAI/v2"}}).Code)
}

func TestGatewayHeaderAuthenticationInferenceScope(t *testing.T) {
	for _, path := range []string{
		"/v1/messages", "/antigravity/v1/messages", "/v1/chat/completions", "/chat/completions",
		"/v1/responses", "/responses/compact", "/backend-api/codex/responses/compact", "/v1/systemone",
		"/v1beta/models/gemini:generateContent", "/v1beta/models/gemini:streamGenerateContent",
		"/antigravity/v1beta/models/gemini:generateContent", "/v1/images/generations", "/v1/images/edits/async",
		"/v1/videos", "/v1/embeddings", "/v1/tts", "/v1/stt", "/api/v3/contents/generations/tasks",
		"/v1/live", "/backend-api/codex/realtime/calls",
	} {
		require.True(t, requiresGatewayHeaderAuthentication(http.MethodPost, path), path)
	}
	for _, path := range []string{"/v1/responses", "/responses", "/backend-api/codex/responses", "/v1/realtime", "/realtime", "/v1/live/call-123", "/backend-api/codex/call-123"} {
		require.True(t, requiresGatewayHeaderAuthentication(http.MethodGet, path), path)
	}
	for _, path := range []string{
		"/v1/models", "/models", "/v1/models/deepseek-chat", "/backend-api/codex/models",
		"/antigravity/models", "/antigravity/v1/models", "/v1beta/models", "/v1beta/models/gemini",
		"/antigravity/v1beta/models/gemini", "/v1/usage", "/v1/sub2api/billing",
		"/v1/images/tasks/task-123", "/v1/images/batches/models", "/v1/images/batches/batch-123/download",
		"/v1/videos/request-123", "/v1/videos/request-123/content", "/v3/contents/generations/tasks/task-123",
		"/v1/custom-voices", "/v1/custom-voices/voice-123/audio",
	} {
		require.False(t, requiresGatewayHeaderAuthentication(http.MethodGet, path), path)
	}
	for _, path := range []string{"/v1/messages/count_tokens", "/messages/count_tokens", "/antigravity/v1/messages/count_tokens", "/v1beta/models/gemini:countTokens", "/v1/images/batches/batch-123/cancel", "/v1/custom-voices", "/custom-voices"} {
		require.False(t, requiresGatewayHeaderAuthentication(http.MethodPost, path), path)
	}
	require.False(t, requiresGatewayHeaderAuthentication(http.MethodDelete, "/v1/images/batches/batch-123"))
	require.False(t, requiresGatewayHeaderAuthentication(http.MethodPatch, "/v1/custom-voices/voice-123"))
}

func TestGatewayHeaderAuthenticationReadOnlySkipsRulesAndSettingsLookup(t *testing.T) {
	// No generation headers, and even settings storage is unavailable: read-only
	// metadata should proceed to API key auth rather than fail this extra gate.
	svc := service.NewSettingService(&gatewayHeaderSettingsRepo{err: errors.New("settings DB unavailable")}, &config.Config{})
	router := headerAuthRouter(t, svc, false)
	for _, path := range []string{"/v1/models", "/models/deepseek-chat", "/backend-api/codex/models", "/v1beta/models/gemini", "/v1/usage", "/v1/images/tasks/task-123"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, w.Code, path)
	}
	w := headerAuthRequest(router, "/v1/messages", http.Header{})
	require.Equal(t, http.StatusServiceUnavailable, w.Code, "real model calls must still fail closed when config cannot be read")
}

func TestGatewayHeaderAuthenticationMetadataBypassesAllConfiguredRules(t *testing.T) {
	svc := service.NewSettingService(&gatewayHeaderSettingsRepo{}, &config.Config{})
	require.NoError(t, svc.SetGatewayHeaderAuthSettings(context.Background(), service.GatewayHeaderAuthSettings{Enabled: true, Rules: []service.GatewayHeaderAuthRule{
		{HeaderName: "User-Agent", RequiredSubstring: "XundaAI"}, {HeaderName: "X-Client-Auth", RequiredSubstring: "allowed-app"},
	}}))
	router := headerAuthRouter(t, svc, false)
	for _, path := range []string{"/v1/models", "/v1beta/models", "/antigravity/v1/models", "/backend-api/codex/models"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("User-Agent", "CC-Switch")
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}
	w := headerAuthRequest(router, "/v1/messages", http.Header{"User-Agent": {"CC-Switch"}})
	require.Equal(t, http.StatusUnauthorized, w.Code)
}
