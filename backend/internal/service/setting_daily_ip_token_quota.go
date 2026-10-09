package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const SettingKeyDailyIPTokenQuota = "daily_ip_token_quota"
const dailyIPTokenQuotaSettingsCacheTTL = 5 * time.Second

// Integers beyond this cannot be represented exactly by the admin UI.
const MaxDailyIPTokenLimit int64 = 9_007_199_254_740_991

type DailyIPTokenQuotaSettings struct {
	Enabled         bool   `json:"enabled"`
	DailyTokenLimit int64  `json:"daily_token_limit"`
	Timezone        string `json:"timezone"`
}

type cachedDailyIPTokenQuotaSettings struct {
	settings  DailyIPTokenQuotaSettings
	expiresAt time.Time
}

func DefaultDailyIPTokenQuotaSettings() DailyIPTokenQuotaSettings {
	return DailyIPTokenQuotaSettings{Enabled: true, DailyTokenLimit: DefaultDailyIPTokenLimit, Timezone: "Asia/Shanghai"}
}

func (v DailyIPTokenQuotaSettings) Validate() error {
	if v.DailyTokenLimit <= 0 || v.DailyTokenLimit > MaxDailyIPTokenLimit {
		return fmt.Errorf("daily_token_limit must be between 1 and %d", MaxDailyIPTokenLimit)
	}
	if strings.TrimSpace(v.Timezone) == "" || len(v.Timezone) > 100 || v.Timezone == "Local" {
		return errors.New("timezone must be an explicit IANA timezone")
	}
	if _, err := time.LoadLocation(v.Timezone); err != nil {
		return fmt.Errorf("invalid timezone: %w", err)
	}
	return nil
}

func (s *SettingService) GetDailyIPTokenQuotaSettings(ctx context.Context) (DailyIPTokenQuotaSettings, error) {
	defaults := DefaultDailyIPTokenQuotaSettings()
	value, err := s.settingRepo.GetValue(ctx, SettingKeyDailyIPTokenQuota)
	if errors.Is(err, ErrSettingNotFound) {
		return defaults, nil
	}
	if err != nil {
		return defaults, fmt.Errorf("read daily IP quota settings: %w", err)
	}
	if strings.TrimSpace(value) == "" {
		return defaults, nil
	}
	if err := json.Unmarshal([]byte(value), &defaults); err != nil {
		return defaults, fmt.Errorf("invalid daily IP quota settings: %w", err)
	}
	if err := defaults.Validate(); err != nil {
		return defaults, err
	}
	return defaults, nil
}

func (s *SettingService) GetDailyIPTokenQuotaSettingsCached(ctx context.Context) (DailyIPTokenQuotaSettings, error) {
	// Serialize refresh and updates: a slow read cannot overwrite a just-saved config.
	s.dailyIPTokenQuotaSettingsMu.Lock()
	defer s.dailyIPTokenQuotaSettingsMu.Unlock()
	if cached := s.dailyIPTokenQuotaSettingsCache; cached != nil && time.Now().Before(cached.expiresAt) {
		return cached.settings, nil
	}
	readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	settings, err := s.GetDailyIPTokenQuotaSettings(readCtx)
	if err != nil {
		return settings, err
	} // Fail closed rather than unexpectedly removing a limit.
	s.dailyIPTokenQuotaSettingsCache = &cachedDailyIPTokenQuotaSettings{settings: settings, expiresAt: time.Now().Add(dailyIPTokenQuotaSettingsCacheTTL)}
	return settings, nil
}

func (s *SettingService) SetDailyIPTokenQuotaSettings(ctx context.Context, settings DailyIPTokenQuotaSettings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	s.dailyIPTokenQuotaSettingsMu.Lock()
	defer s.dailyIPTokenQuotaSettingsMu.Unlock()
	if err := s.settingRepo.Set(ctx, SettingKeyDailyIPTokenQuota, string(data)); err != nil {
		return err
	}
	s.dailyIPTokenQuotaSettingsCache = &cachedDailyIPTokenQuotaSettings{settings: settings, expiresAt: time.Now().Add(dailyIPTokenQuotaSettingsCacheTTL)}
	return nil
}
