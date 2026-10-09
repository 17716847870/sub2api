package middleware

import (
	"net/http"
	"strings"
)

// Model discovery only returns metadata. A valid active key must be able to
// discover models before topping up or subscribing; generation remains billable.
func isModelDiscoveryRequest(method, path string) bool {
	if method != http.MethodGet {
		return false
	}
	for _, prefix := range []string{
		"/v1/models", "/models", "/backend-api/codex/models",
		"/antigravity/models", "/antigravity/v1/models",
		"/v1beta/models", "/antigravity/v1beta/models",
	} {
		if path == prefix {
			return true
		}
		if strings.HasPrefix(path, prefix+"/") {
			model := strings.TrimPrefix(path, prefix+"/")
			return model != "" && !strings.Contains(model, "/")
		}
	}
	return false
}
