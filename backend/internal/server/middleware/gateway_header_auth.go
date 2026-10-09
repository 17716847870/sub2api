package middleware

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const gatewayHeaderAuthOriginalUAKey = "gateway_header_auth_original_user_agent"
const GatewayHeaderAuthFailedCode = "gateway_header_auth_failed"

// Read-only discovery and task bookkeeping must remain available to a valid
// key without client-specific headers. WebSocket conversation entrypoints are
// GET routes but still initiate/attach to model inference, so retain the gate.
func requiresGatewayHeaderAuthentication(method, path string) bool {
	codexDirect := strings.HasPrefix(path, "/backend-api/codex/")
	for _, prefix := range []string{"/backend-api/codex", "/antigravity/v1beta", "/antigravity/v1", "/v1beta", "/api/v3", "/v3", "/v1"} {
		if strings.HasPrefix(path, prefix+"/") {
			path = strings.TrimPrefix(path, prefix)
			break
		}
	}
	if method == http.MethodGet {
		if path == "/responses" || path == "/realtime" || strings.HasPrefix(path, "/live/") {
			return true
		}
		// Codex live sideband is registered as GET /backend-api/codex/:call_id.
		return codexDirect && path != "/models" && len(path) > 1 && !strings.Contains(path[1:], "/")
	}
	if method != http.MethodPost {
		return false
	}
	if path == "/messages/count_tokens" || path == "/custom-voices" {
		return false
	}
	if strings.HasPrefix(path, "/models/") && strings.HasSuffix(path, ":countTokens") {
		return false
	}
	if strings.HasPrefix(path, "/images/batches/") && strings.HasSuffix(path, "/cancel") {
		return false
	}
	return true
}

// GatewayHeaderAuthentication is mounted only on gateway routes, before API key
// authentication and upstream scheduling for model calls. Discovery and other
// read-only routes retain API key authentication but bypass this extra gate.
func GatewayHeaderAuthentication(settings *service.SettingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requiresGatewayHeaderAuthentication(c.Request.Method, c.Request.URL.Path) || settings == nil {
			c.Next()
			return
		}
		rule, err := settings.GetGatewayHeaderAuthSettingsCached(c.Request.Context())
		if err != nil {
			c.Header("Retry-After", "5")
			gatewayHeaderAuthError(c, http.StatusServiceUnavailable, "gateway_header_auth_unavailable", "Request header authentication is temporarily unavailable. Please retry later.")
			return
		}
		if !rule.Enabled {
			c.Next()
			return
		}
		for _, required := range rule.Rules {
			values := c.Request.Header.Values(required.HeaderName)
			if strings.EqualFold(required.HeaderName, "User-Agent") {
				if original, exists := c.Get(gatewayHeaderAuthOriginalUAKey); exists {
					if rawValues, ok := original.([]string); ok {
						values = rawValues
					}
				}
			}
			matched := false
			for _, value := range values {
				// Each configured rule must match. Multiple received values are
				// alternatives within that one rule, not alternatives across rules.
				if strings.Contains(value, required.RequiredSubstring) {
					matched = true
					break
				}
			}
			if !matched {
				MarkIngressRejected(c, IngressRejectGatewayHeaderAuth)
				service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalPolicyDenied)
				gatewayHeaderAuthError(c, http.StatusUnauthorized, GatewayHeaderAuthFailedCode, "Request header authentication failed.")
				return
			}
		}
		c.Next()
	}
}

func gatewayHeaderAuthError(c *gin.Context, status int, code, message string) {
	path := c.Request.URL.Path
	if strings.HasPrefix(path, "/v1beta/") || strings.HasPrefix(path, "/antigravity/v1beta/") {
		abortWithGoogleError(c, status, message)
		return
	}
	errorType := "authentication_error"
	if status == http.StatusServiceUnavailable {
		errorType = "api_error"
	}
	body := gin.H{"error": gin.H{"type": errorType, "code": code, "message": message}}
	if strings.Contains(path, "/messages") {
		body["type"] = "error"
	}
	c.AbortWithStatusJSON(status, body)
}
