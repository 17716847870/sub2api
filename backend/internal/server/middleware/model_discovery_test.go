//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIsModelDiscoveryRequest(t *testing.T) {
	for _, prefix := range []string{"/v1/models", "/models", "/backend-api/codex/models", "/antigravity/models", "/antigravity/v1/models", "/v1beta/models", "/antigravity/v1beta/models"} {
		require.True(t, isModelDiscoveryRequest(http.MethodGet, prefix))
		require.True(t, isModelDiscoveryRequest(http.MethodGet, prefix+"/gpt-test"))
		require.False(t, isModelDiscoveryRequest(http.MethodPost, prefix))
		require.False(t, isModelDiscoveryRequest(http.MethodPost, prefix+"/gemini:generateContent"))
		require.False(t, isModelDiscoveryRequest(http.MethodGet, prefix+"/gpt-test/extra"))
	}
	for _, path := range []string{"/v1/messages", "/v1/responses", "/v1/models-extra", "/api/v1/models", "/api/v1/admin/models"} {
		require.False(t, isModelDiscoveryRequest(http.MethodGet, path))
	}
}

func modelDiscoveryAuthRouter(t *testing.T, key *service.APIKey, google bool, subscriptions *service.SubscriptionService) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{RunMode: config.RunModeStandard}
	repo := fakeAPIKeyRepo{getByKey: func(_ context.Context, token string) (*service.APIKey, error) {
		if token != key.Key {
			return nil, service.ErrAPIKeyNotFound
		}
		copy := *key
		return &copy, nil
	}}
	svc := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)
	r := gin.New()
	if google {
		r.Use(APIKeyAuthWithSubscriptionGoogle(svc, subscriptions, cfg))
	} else {
		r.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(svc, subscriptions, cfg)))
	}
	r.Any("/*path", func(c *gin.Context) {
		got, ok := GetAPIKeyFromContext(c)
		require.True(t, ok)
		require.Equal(t, key.ID, got.ID)
		c.Status(http.StatusOK)
	})
	return r
}

func modelDiscoveryKey() *service.APIKey {
	group := &service.Group{ID: 10, Status: service.StatusActive, Hydrated: true, Platform: service.PlatformOpenAI, SubscriptionType: service.SubscriptionTypeStandard}
	user := &service.User{ID: 7, Status: service.StatusActive, Role: service.RoleUser, Balance: 0}
	return &service.APIKey{ID: 20, Key: "model-discovery-test-key", UserID: user.ID, User: user, GroupID: &group.ID, Group: group, Status: service.StatusActive}
}
func requestModelDiscovery(r *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestModelDiscoveryAllowsZeroBalanceButGenerationStillRejected(t *testing.T) {
	for _, google := range []bool{false, true} {
		key := modelDiscoveryKey()
		r := modelDiscoveryAuthRouter(t, key, google, nil)
		for _, path := range []string{"/models", "/v1/models", "/v1/models/gpt-test", "/backend-api/codex/models", "/antigravity/v1/models", "/v1beta/models", "/v1beta/models/gemini-test"} {
			w := requestModelDiscovery(r, http.MethodGet, path, key.Key)
			require.Equal(t, http.StatusOK, w.Code, path+" "+w.Body.String())
		}
		for _, path := range []string{"/v1/responses", "/v1/messages", "/v1beta/models/gemini:generateContent"} {
			w := requestModelDiscovery(r, http.MethodPost, path, key.Key)
			require.Equal(t, http.StatusForbidden, w.Code)
			require.Contains(t, w.Body.String(), "Insufficient account balance")
		}
	}
}

func TestModelDiscoveryAllowsSpentQuotaButGenerationStillRejected(t *testing.T) {
	for _, google := range []bool{false, true} {
		key := modelDiscoveryKey()
		key.Quota, key.QuotaUsed = 1, 1
		key.Status = service.StatusAPIKeyQuotaExhausted
		r := modelDiscoveryAuthRouter(t, key, google, nil)
		require.Equal(t, http.StatusOK, requestModelDiscovery(r, http.MethodGet, "/v1/models", key.Key).Code)
		require.Equal(t, http.StatusTooManyRequests, requestModelDiscovery(r, http.MethodPost, "/v1/responses", key.Key).Code)
	}
}

func TestModelDiscoveryKeepsSecurityChecks(t *testing.T) {
	for _, google := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			change func(*service.APIKey)
			status int
		}{
			{"disabled key", func(k *service.APIKey) { k.Status = "disabled" }, http.StatusUnauthorized},
			{"inactive user", func(k *service.APIKey) { k.User.Status = "disabled" }, http.StatusUnauthorized},
			{"disabled group", func(k *service.APIKey) { k.Group.Status = "disabled" }, http.StatusForbidden},
			{"expired status", func(k *service.APIKey) { k.Status = service.StatusAPIKeyExpired }, http.StatusForbidden},
			{"expired time", func(k *service.APIKey) { expired := time.Now().Add(-time.Hour); k.ExpiresAt = &expired }, http.StatusForbidden},
			{"IP whitelist", func(k *service.APIKey) { k.IPWhitelist = []string{"203.0.113.8"} }, http.StatusForbidden},
		} {
			t.Run(tc.name, func(t *testing.T) {
				key := modelDiscoveryKey()
				tc.change(key)
				r := modelDiscoveryAuthRouter(t, key, google, nil)
				w := requestModelDiscovery(r, http.MethodGet, "/v1/models", key.Key)
				require.Equal(t, tc.status, w.Code, w.Body.String())
			})
		}
		key := modelDiscoveryKey()
		r := modelDiscoveryAuthRouter(t, key, google, nil)
		require.Equal(t, http.StatusUnauthorized, requestModelDiscovery(r, http.MethodGet, "/v1/models", "").Code)
		require.Equal(t, http.StatusUnauthorized, requestModelDiscovery(r, http.MethodGet, "/v1/models", "invalid-key").Code)
	}
}

func TestModelDiscoveryNeedsNoSubscriptionButGenerationDoes(t *testing.T) {
	for _, google := range []bool{false, true} {
		key := modelDiscoveryKey()
		key.Group.SubscriptionType = service.SubscriptionTypeSubscription
		calls := 0
		subscriptions := service.NewSubscriptionService(nil, fakeGoogleSubscriptionRepo{getActive: func(context.Context, int64, int64) (*service.UserSubscription, error) {
			calls++
			return nil, service.ErrSubscriptionNotFound
		}}, nil, nil, &config.Config{})
		t.Cleanup(subscriptions.Stop)
		r := modelDiscoveryAuthRouter(t, key, google, subscriptions)
		require.Equal(t, http.StatusOK, requestModelDiscovery(r, http.MethodGet, "/v1/models", key.Key).Code)
		require.Zero(t, calls)
		w := requestModelDiscovery(r, http.MethodPost, "/v1/responses", key.Key)
		require.Equal(t, http.StatusForbidden, w.Code)
		require.Contains(t, w.Body.String(), "No active subscription found")
		require.Equal(t, 1, calls)
	}
}
