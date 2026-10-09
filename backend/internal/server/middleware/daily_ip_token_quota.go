package middleware

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type DailyIPTokenQuotaMiddleware gin.HandlerFunc

func NewDailyIPTokenQuotaMiddleware(quota *service.DailyIPTokenQuotaService) DailyIPTokenQuotaMiddleware {
	return func(c *gin.Context) {
		if !quota.Enabled() {
			c.Next()
			return
		}
		clientIP := ip.GetTrustedClientIP(c)
		// Quota admission and all downstream usage records must use the same IP.
		// Never let the legacy raw forwarding-header compatibility mode override it.
		ip.SetForwardedIPSettings(c, false, nil)
		c.Request = c.Request.WithContext(service.WithDailyIPTokenQuota(c.Request.Context(), quota, clientIP))
		if !dailyIPTokenQuotaBillableRequest(c.Request) {
			c.Next()
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		status, err := quota.Check(ctx, clientIP)
		cancel()
		if err == nil {
			c.Next()
			return
		}
		errorType, errorCode := "api_error", "daily_ip_token_quota_unavailable"
		code, message := http.StatusServiceUnavailable, "Daily IP token quota is temporarily unavailable; please retry later."
		c.Header("Retry-After", "5")
		if errors.Is(err, service.ErrDailyIPTokenQuotaExceeded) {
			MarkIngressRejected(c, IngressRejectDailyIPTokenQuota)
			service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalPolicyDenied)
			errorType, errorCode = "rate_limit_error", "daily_ip_token_limit_exceeded"
			code, message = http.StatusTooManyRequests, status.Message()
			seconds := int64(math.Ceil(time.Until(status.ResetAt).Seconds()))
			if seconds < 1 {
				seconds = 1
			}
			c.Header("Retry-After", strconv.FormatInt(seconds, 10))
			c.Header("X-Daily-Token-Limit", strconv.FormatInt(status.Limit, 10))
			c.Header("X-Daily-Token-Used", strconv.FormatInt(status.Used, 10))
			c.Header("X-Daily-Token-Reset", strconv.FormatInt(status.ResetAt.Unix(), 10))
		}
		if strings.Contains(c.Request.URL.Path, "/v1beta/") {
			abortWithGoogleError(c, code, message)
			return
		}
		c.AbortWithStatusJSON(code, gin.H{"error": gin.H{"type": errorType, "code": errorCode, "message": message}})
	}
}

func dailyIPTokenQuotaBillableRequest(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return true
	}
	if r.Method != http.MethodPost {
		return false
	}
	// Read-only token counting and cancellation remain usable when quota is exhausted.
	return !strings.HasSuffix(r.URL.Path, "/count_tokens") && !strings.HasSuffix(r.URL.Path, "/cancel")
}
