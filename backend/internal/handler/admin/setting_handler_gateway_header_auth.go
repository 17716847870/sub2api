package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) GetGatewayHeaderAuthSettings(c *gin.Context) {
	settings, err := h.settingService.GetGatewayHeaderAuthSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *SettingHandler) UpdateGatewayHeaderAuthSettings(c *gin.Context) {
	var req struct {
		Enabled *bool                            `json:"enabled"`
		Rules   *[]service.GatewayHeaderAuthRule `json:"rules"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.Enabled == nil || req.Rules == nil {
		response.BadRequest(c, "enabled and rules are required")
		return
	}
	settings := service.GatewayHeaderAuthSettings{Enabled: *req.Enabled, Rules: *req.Rules}
	for i := range settings.Rules {
		settings.Rules[i].HeaderName = strings.TrimSpace(settings.Rules[i].HeaderName)
	}
	if err := settings.Validate(); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.settingService.SetGatewayHeaderAuthSettings(c.Request.Context(), settings); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	updated, err := h.settingService.GetGatewayHeaderAuthSettingsCached(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}
