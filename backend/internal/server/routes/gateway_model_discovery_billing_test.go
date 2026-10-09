package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

type discoveryRouteKeyRepo struct{ keyBillingRouteAPIKeyRepo }

func (*discoveryRouteKeyRepo) UpdateLastUsed(context.Context, int64, time.Time) error { return nil }

type discoveryRouteAccounts struct{ service.AccountRepository }

func (discoveryRouteAccounts) ListSchedulableByGroupID(context.Context, int64) ([]service.Account, error) {
	return nil, nil
}

func TestGatewayModelDiscoveryZeroBalanceEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	group := &service.Group{ID: 42, Status: service.StatusActive, Hydrated: true, Platform: service.PlatformDeepseek, SubscriptionType: service.SubscriptionTypeStandard}
	user := &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive, Balance: 0}
	key := &service.APIKey{ID: 100, UserID: user.ID, Key: "discovery-route-key", Status: service.StatusActive, User: user, GroupID: &group.ID, Group: group}
	cfg := &config.Config{RunMode: config.RunModeStandard, Gateway: config.GatewayConfig{MaxBodySize: 1024 * 1024, TextMaxBodySize: 1024 * 1024}}
	keyService := service.NewAPIKeyService(&discoveryRouteKeyRepo{keyBillingRouteAPIKeyRepo{apiKey: key}}, nil, nil, nil, nil, nil, cfg)
	gateway := service.NewGatewayService(discoveryRouteAccounts{}, nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := &handler.Handlers{
		Gateway:       handler.NewGatewayHandler(gateway, nil, nil, nil, nil, nil, nil, nil, keyService, nil, nil, nil, nil, cfg, nil),
		OpenAIGateway: &handler.OpenAIGatewayHandler{}, AsyncImage: handler.NewAsyncImageHandler(nil, nil),
	}
	r := gin.New()
	settings := service.NewSettingService(&gatewayHeaderRouteSettingsRepo{}, cfg)
	RegisterGatewayRoutes(r, h, servermiddleware.NewAPIKeyAuthMiddleware(keyService, nil, cfg), keyService, nil, nil, settings, nil, cfg, nil)
	// Header authentication is enabled and the caller supplies no XundaAI marker.
	for _, path := range []string{"/v1/models", "/models"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+key.Key)
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var response struct {
			Object string `json:"object"`
			Data   []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.Equal(t, "list", response.Object)
		require.NotEmpty(t, response.Data, "model metadata should be available before topping up")
	}
	for _, credential := range []string{"", "wrong-key"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		if credential != "" {
			req.Header.Set("Authorization", "Bearer "+credential)
		}
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.NotContains(t, w.Body.String(), "gateway_header_auth_failed")
	}
	// A model call still needs the configured client header, even with a valid key.
	missingHeader := httptest.NewRecorder()
	missingHeaderRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	missingHeaderRequest.Header.Set("Authorization", "Bearer "+key.Key)
	r.ServeHTTP(missingHeader, missingHeaderRequest)
	require.Equal(t, http.StatusUnauthorized, missingHeader.Code)
	require.Contains(t, missingHeader.Body.String(), "gateway_header_auth_failed")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Authorization", "Bearer "+key.Key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "XundaAI/1.0")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "INSUFFICIENT_BALANCE", "generation must still require funds")
}
