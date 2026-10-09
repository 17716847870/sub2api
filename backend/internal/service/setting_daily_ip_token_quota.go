package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

const SettingKeyDailyIPTokenQuota = "daily_ip_token_quota"
const dailyIPTokenQuotaSettingsCacheTTL = 5 * time.Second

// Integers beyond this cannot be represented exactly by the admin UI.
const MaxDailyIPTokenLimit int64 = 9_007_199_254_740_991

const MaxDailyIPWhitelistEntries = 1000

type DailyIPTokenQuotaSettings struct {
	Enabled                  bool     `json:"enabled"`
	DailyTokenLimit          int64    `json:"daily_token_limit"`
	Timezone                 string   `json:"timezone"`
	WhitelistDailyTokenLimit int64    `json:"whitelist_daily_token_limit"`
	Whitelist                []string `json:"whitelist"`
}

type cachedDailyIPTokenQuotaSettings struct {
	settings  DailyIPTokenQuotaSettings
	expiresAt time.Time
}

func DefaultDailyIPTokenQuotaSettings() DailyIPTokenQuotaSettings {
	return DailyIPTokenQuotaSettings{Enabled: true, DailyTokenLimit: DefaultDailyIPTokenLimit, Timezone: "Asia/Shanghai", Whitelist: []string{}}
}

func (v DailyIPTokenQuotaSettings) Validate() error {
	if v.DailyTokenLimit < 0 || v.DailyTokenLimit > MaxDailyIPTokenLimit {
		return fmt.Errorf("daily_token_limit must be between 0 and %d", MaxDailyIPTokenLimit)
	}
	if v.WhitelistDailyTokenLimit < 0 || v.WhitelistDailyTokenLimit > MaxDailyIPTokenLimit {
		return fmt.Errorf("whitelist_daily_token_limit must be between 0 and %d", MaxDailyIPTokenLimit)
	}
	if strings.TrimSpace(v.Timezone) == "" || len(v.Timezone) > 100 || v.Timezone == "Local" {
		return errors.New("timezone must be an explicit IANA timezone")
	}
	if _, err := time.LoadLocation(v.Timezone); err != nil {
		return fmt.Errorf("invalid timezone: %w", err)
	}
	if len(v.Whitelist) > MaxDailyIPWhitelistEntries {
		return fmt.Errorf("at most %d whitelist IPs are allowed", MaxDailyIPWhitelistEntries)
	}
	seen := make(map[string]struct{}, len(v.Whitelist))
	for i, entry := range v.Whitelist {
		address, err := normalizeDailyQuotaIP(entry)
		if err != nil {
			return fmt.Errorf("whitelist entry %d: invalid IP address", i+1)
		}
		if _, exists := seen[address]; exists {
			return fmt.Errorf("whitelist entry %d: duplicate IP address", i+1)
		}
		seen[address] = struct{}{}
	}
	return nil
}

func normalizeDailyQuotaIP(raw string) (string, error) {
	address, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil || address.Zone() != "" {
		return "", errors.New("invalid IP address")
	}
	return address.Unmap().String(), nil
}

// Copy and normalize entries before storage/cache to keep equivalent IPv6 and
// IPv4-mapped addresses consistent with the gateway's trusted client IP.
func normalizeDailyIPTokenQuotaSettings(settings DailyIPTokenQuotaSettings) DailyIPTokenQuotaSettings {
	entries := make([]string, len(settings.Whitelist))
	copy(entries, settings.Whitelist)
	for i := range entries {
		entries[i], _ = normalizeDailyQuotaIP(entries[i])
	}
	settings.Whitelist = entries
	return settings
}

func (v DailyIPTokenQuotaSettings) LimitForIP(clientIP string) (int64, bool) {
	for _, entry := range v.Whitelist {
		if entry == clientIP {
			return v.WhitelistDailyTokenLimit, true
		}
	}
	return v.DailyTokenLimit, false
}

func (v DailyIPTokenQuotaSettings) HasAnyLimit() bool {
	if !v.Enabled {
		return false
	}
	if v.DailyTokenLimit > 0 {
		return true
	}
	return len(v.Whitelist) > 0 && v.WhitelistDailyTokenLimit > 0
}

func decodeDailyIPTokenQuotaSettings(value string) (DailyIPTokenQuotaSettings, error) {
	settings := DefaultDailyIPTokenQuotaSettings()
	// Decode scalar config separately so the prior object whitelist is readable.
	type plainSettings DailyIPTokenQuotaSettings
	var stored struct {
		*plainSettings
		Whitelist json.RawMessage `json:"whitelist"`
	}
	stored.plainSettings = (*plainSettings)(&settings)
	if err := json.Unmarshal([]byte(value), &stored); err != nil {
		return settings, err
	}
	if len(stored.Whitelist) > 0 && string(stored.Whitelist) != "null" {
		if err := json.Unmarshal(stored.Whitelist, &settings.Whitelist); err != nil {
			var legacy []struct {
				IPAddress       string `json:"ip_address"`
				DailyTokenLimit *int64 `json:"daily_token_limit"`
			}
			if err := json.Unmarshal(stored.Whitelist, &legacy); err != nil {
				return settings, err
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(value), &fields); err != nil {
				return settings, err
			}
			settings.Whitelist = make([]string, len(legacy))
			for i, entry := range legacy {
				if entry.DailyTokenLimit == nil || *entry.DailyTokenLimit < 0 || *entry.DailyTokenLimit > MaxDailyIPTokenLimit {
					return settings, errors.New("invalid legacy whitelist quota")
				}
				settings.Whitelist[i] = entry.IPAddress
				if _, explicit := fields["whitelist_daily_token_limit"]; !explicit && *entry.DailyTokenLimit > 0 && (settings.WhitelistDailyTokenLimit == 0 || *entry.DailyTokenLimit < settings.WhitelistDailyTokenLimit) {
					settings.WhitelistDailyTokenLimit = *entry.DailyTokenLimit
				}
			}
		}
	}
	return settings, nil
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
	defaults, err = decodeDailyIPTokenQuotaSettings(value)
	if err != nil {
		return defaults, fmt.Errorf("invalid daily IP quota settings: %w", err)
	}
	if err := defaults.Validate(); err != nil {
		return defaults, err
	}
	return normalizeDailyIPTokenQuotaSettings(defaults), nil
}

func (s *SettingService) GetDailyIPTokenQuotaSettingsCached(ctx context.Context) (DailyIPTokenQuotaSettings, error) {
	// Serialize refresh and updates: a slow read cannot overwrite a just-saved config.
	s.dailyIPTokenQuotaSettingsMu.Lock()
	defer s.dailyIPTokenQuotaSettingsMu.Unlock()
	if cached := s.dailyIPTokenQuotaSettingsCache; cached != nil && time.Now().Before(cached.expiresAt) {
		return normalizeDailyIPTokenQuotaSettings(cached.settings), nil
	}
	readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	settings, err := s.GetDailyIPTokenQuotaSettings(readCtx)
	if err != nil {
		return settings, err
	} // Fail closed rather than unexpectedly removing a limit.
	s.dailyIPTokenQuotaSettingsCache = &cachedDailyIPTokenQuotaSettings{settings: normalizeDailyIPTokenQuotaSettings(settings), expiresAt: time.Now().Add(dailyIPTokenQuotaSettingsCacheTTL)}
	return settings, nil
}

func (s *SettingService) SetDailyIPTokenQuotaSettings(ctx context.Context, settings DailyIPTokenQuotaSettings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	settings = normalizeDailyIPTokenQuotaSettings(settings)
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	s.dailyIPTokenQuotaSettingsMu.Lock()
	defer s.dailyIPTokenQuotaSettingsMu.Unlock()
	if err := s.settingRepo.Set(ctx, SettingKeyDailyIPTokenQuota, string(data)); err != nil {
		return err
	}
	s.dailyIPTokenQuotaSettingsCache = &cachedDailyIPTokenQuotaSettings{settings: normalizeDailyIPTokenQuotaSettings(settings), expiresAt: time.Now().Add(dailyIPTokenQuotaSettingsCacheTTL)}
	return nil
}
