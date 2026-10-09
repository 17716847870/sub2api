package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) SetDailyIPTokenQuotaService(quota *service.DailyIPTokenQuotaService) {
	h.dailyIPTokenQuota = quota
}

func (h *SettingHandler) GetDailyIPTokenQuotaSettings(c *gin.Context) {
	settings, err := h.settingService.GetDailyIPTokenQuotaSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *SettingHandler) UpdateDailyIPTokenQuotaSettings(c *gin.Context) {
	var req struct {
		Enabled                  *bool     `json:"enabled"`
		DailyTokenLimit          *int64    `json:"daily_token_limit"`
		Timezone                 *string   `json:"timezone"`
		WhitelistDailyTokenLimit *int64    `json:"whitelist_daily_token_limit"`
		Whitelist                *[]string `json:"whitelist"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.Enabled == nil || req.DailyTokenLimit == nil || req.Timezone == nil {
		response.BadRequest(c, "enabled, daily_token_limit and timezone are required")
		return
	}
	settings := service.DailyIPTokenQuotaSettings{Enabled: *req.Enabled, DailyTokenLimit: *req.DailyTokenLimit, Timezone: strings.TrimSpace(*req.Timezone)}
	if req.Whitelist == nil || req.WhitelistDailyTokenLimit == nil {
		// Clients saving just ordinary settings must preserve whitelist policy.
		previous, err := h.settingService.GetDailyIPTokenQuotaSettings(c.Request.Context())
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		settings.Whitelist = previous.Whitelist
		settings.WhitelistDailyTokenLimit = previous.WhitelistDailyTokenLimit
	}
	if req.Whitelist != nil {
		settings.Whitelist = *req.Whitelist
	}
	if req.WhitelistDailyTokenLimit != nil {
		settings.WhitelistDailyTokenLimit = *req.WhitelistDailyTokenLimit
	}
	if err := settings.Validate(); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.settingService.SetDailyIPTokenQuotaSettings(c.Request.Context(), settings); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	updated, err := h.settingService.GetDailyIPTokenQuotaSettingsCached(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}

func (h *SettingHandler) ListDailyIPLimitedIPs(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 || page > 1_000_000 {
		response.BadRequest(c, "Invalid page")
		return
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		response.BadRequest(c, "page_size must be between 1 and 100")
		return
	}
	search := strings.TrimSpace(c.Query("search"))
	if len(search) > 45 {
		response.BadRequest(c, "IP search is too long")
		return
	}
	result, err := h.dailyIPTokenQuota.ListLimitedIPs(c.Request.Context(), search, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
