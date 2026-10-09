package middleware

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const gatewayHeaderAuthOriginalUAKey = "gateway_header_auth_original_user_agent"
const GatewayHeaderAuthFailedCode = "gateway_header_auth_failed"

// GatewayHeaderAuthentication is mounted only on gateway routes, before API key
// authentication and upstream scheduling. It does not affect the admin panel.
func GatewayHeaderAuthentication(settings *service.SettingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if settings == nil { // Isolated route fixtures have no settings service.
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
