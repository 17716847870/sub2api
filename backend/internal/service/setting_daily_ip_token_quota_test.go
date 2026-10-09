package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestDailyIPTokenQuotaSettingsDefaultsAndPersistence(t *testing.T) {
	repo := &panelRateLimitSettingRepo{values: map[string]string{}}
	settings := NewSettingService(repo, &config.Config{Timezone: "UTC"})
	ctx := context.Background()
	got, err := settings.GetDailyIPTokenQuotaSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, DefaultDailyIPTokenQuotaSettings(), got, "deployment timezone must not override quota UI settings")
	got.Timezone, got.DailyTokenLimit = "Asia/Tokyo", 250_000_000
	require.NoError(t, settings.SetDailyIPTokenQuotaSettings(ctx, got))
	restarted := NewSettingService(repo, &config.Config{Timezone: "America/New_York"})
	stored, err := restarted.GetDailyIPTokenQuotaSettingsCached(ctx)
	require.NoError(t, err)
	require.Equal(t, got, stored)
	require.Contains(t, repo.values[SettingKeyDailyIPTokenQuota], `"daily_token_limit":250000000`)
}

func TestDailyIPTokenQuotaSettingsHotUpdateAndFailClosed(t *testing.T) {
	repo := &panelRateLimitSettingRepo{values: map[string]string{}}
	settings := NewSettingService(repo, &config.Config{})
	usage := &dailyIPTokenUsageStub{used: 100_000_000}
	quota, err := NewDailyIPTokenQuotaService(usage, settings)
	require.NoError(t, err)
	ctx := context.Background()
	_, err = quota.Check(ctx, "203.0.113.8")
	require.ErrorIs(t, err, ErrDailyIPTokenQuotaExceeded)
	updated := DefaultDailyIPTokenQuotaSettings()
	updated.DailyTokenLimit = 200_000_000
	require.NoError(t, settings.SetDailyIPTokenQuotaSettings(ctx, updated))
	status, err := quota.Check(ctx, "203.0.113.8")
	require.NoError(t, err)
	require.Equal(t, int64(200_000_000), status.Limit)
	updated.Enabled = false
	require.NoError(t, settings.SetDailyIPTokenQuotaSettings(ctx, updated))
	usage.err = errors.New("usage DB unavailable")
	_, err = quota.Check(ctx, "invalid")
	require.NoError(t, err, "disabled quota should not query token usage")
	updated.Enabled = true
	require.NoError(t, settings.SetDailyIPTokenQuotaSettings(ctx, updated))
	settings.dailyIPTokenQuotaSettingsCache.expiresAt = time.Time{}
	repo.getValueErr = errors.New("settings DB unavailable")
	_, err = quota.Check(ctx, "203.0.113.8")
	require.ErrorContains(t, err, "settings DB unavailable")
}

func TestDailyIPTokenQuotaSettingsValidation(t *testing.T) {
	for _, value := range []DailyIPTokenQuotaSettings{
		{Enabled: true, DailyTokenLimit: -1, Timezone: "Asia/Shanghai"},
		{Enabled: true, DailyTokenLimit: MaxDailyIPTokenLimit + 1, Timezone: "Asia/Shanghai"},
		{Enabled: true, DailyTokenLimit: 100, Timezone: "Invalid/Zone"},
		{Enabled: true, DailyTokenLimit: 100, Timezone: "Local"},
		{Enabled: false, DailyTokenLimit: 100, Timezone: ""},
	} {
		require.Error(t, value.Validate())
	}
	repo := &panelRateLimitSettingRepo{values: map[string]string{SettingKeyDailyIPTokenQuota: "broken-json"}}
	settings := NewSettingService(repo, &config.Config{})
	_, err := settings.GetDailyIPTokenQuotaSettings(context.Background())
	require.Error(t, err, "corrupt stored config must not silently disable limits")
}

type dailyIPListerStub struct {
	items          []DailyIPLimitedIP
	total          int64
	limit          int64
	whitelist      []string
	whitelistLimit int64
	calls          int
	start, end     time.Time
}

func (r *dailyIPListerStub) GetTokenUsageByIP(context.Context, string, time.Time, time.Time) (int64, error) {
	return 0, nil
}
func (r *dailyIPListerStub) ListLimitedIPs(_ context.Context, start, end time.Time, limit, whitelistLimit int64, whitelist []string, _ string, _, _ int) ([]DailyIPLimitedIP, int64, error) {
	r.limit, r.start, r.end = limit, start, end
	r.whitelist = whitelist
	r.whitelistLimit = whitelistLimit
	r.calls++
	return r.items, r.total, nil
}
func TestDailyIPTokenQuotaListingUsesLiveSettings(t *testing.T) {
	repo := &panelRateLimitSettingRepo{values: map[string]string{}}
	settings := NewSettingService(repo, &config.Config{})
	lister := &dailyIPListerStub{items: []DailyIPLimitedIP{{IPAddress: "203.0.113.8", UsedTokens: 150_000_000}}, total: 1}
	quota, err := NewDailyIPTokenQuotaService(lister, settings)
	require.NoError(t, err)
	quota.now = func() time.Time { return time.Date(2026, 10, 8, 16, 0, 0, 0, time.UTC) }
	result, err := quota.ListLimitedIPs(context.Background(), "", 1, 20)
	require.NoError(t, err)
	require.Equal(t, "2026-10-09T00:00:00+08:00", result.DayStart.Format(time.RFC3339))
	require.Equal(t, "2026-10-10T00:00:00+08:00", result.Items[0].ResetAt.Format(time.RFC3339))
	updated := DefaultDailyIPTokenQuotaSettings()
	updated.Timezone = "UTC"
	updated.DailyTokenLimit = 200_000_000
	require.NoError(t, settings.SetDailyIPTokenQuotaSettings(context.Background(), updated))
	_, err = quota.ListLimitedIPs(context.Background(), "", 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(200_000_000), lister.limit)
	require.Equal(t, "2026-10-08T00:00:00Z", lister.start.Format(time.RFC3339))
	updated.Enabled = false
	require.NoError(t, settings.SetDailyIPTokenQuotaSettings(context.Background(), updated))
	result, err = quota.ListLimitedIPs(context.Background(), "", 1, 20)
	require.NoError(t, err)
	require.Empty(t, result.Items)
	require.Zero(t, result.Total)
}

func TestDailyIPQuotaWhitelistEffectiveLimitsAndZero(t *testing.T) {
	ctx := context.Background()
	repo := &panelRateLimitSettingRepo{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	usage := &dailyIPTokenUsageStub{used: 150_000_000}
	quota, err := NewDailyIPTokenQuotaService(usage, svc)
	require.NoError(t, err)
	settings := DefaultDailyIPTokenQuotaSettings()
	settings.Whitelist = []string{"203.0.113.8", "203.0.113.9", "2001:0db8:0:0::10"}
	settings.WhitelistDailyTokenLimit = 200_000_000
	require.NoError(t, svc.SetDailyIPTokenQuotaSettings(ctx, settings))
	for _, address := range []string{"203.0.113.8", "203.0.113.9", "2001:db8::10"} {
		status, err := quota.Check(ctx, address)
		require.NoError(t, err)
		require.Equal(t, int64(200_000_000), status.Limit, "every whitelist IP uses the same quota")
	}
	status, err := quota.Check(ctx, "203.0.113.11")
	require.ErrorIs(t, err, ErrDailyIPTokenQuotaExceeded)
	require.Equal(t, DefaultDailyIPTokenLimit, status.Limit)
	settings.WhitelistDailyTokenLimit = 50_000_000
	settings.DailyTokenLimit = 0
	require.NoError(t, svc.SetDailyIPTokenQuotaSettings(ctx, settings))
	for _, address := range []string{"203.0.113.8", "203.0.113.9", "2001:db8::10"} {
		status, err := quota.Check(ctx, address)
		require.ErrorIs(t, err, ErrDailyIPTokenQuotaExceeded)
		require.Equal(t, int64(50_000_000), status.Limit)
	}
	before := usage.calls
	_, err = quota.Check(ctx, "203.0.113.11")
	require.NoError(t, err)
	require.Equal(t, before, usage.calls)
	settings.WhitelistDailyTokenLimit = 0
	settings.DailyTokenLimit = DefaultDailyIPTokenLimit
	require.NoError(t, svc.SetDailyIPTokenQuotaSettings(ctx, settings))
	for _, address := range []string{"203.0.113.8", "203.0.113.9", "2001:db8::10"} {
		status, err := quota.Check(ctx, address)
		require.NoError(t, err)
		require.Zero(t, status.Limit)
	}
	require.Equal(t, before, usage.calls, "zero whitelist quota bypasses usage queries for every whitelist IP")
	settings.Enabled = false
	require.NoError(t, svc.SetDailyIPTokenQuotaSettings(ctx, settings))
	_, err = quota.Check(ctx, "203.0.113.11")
	require.NoError(t, err)
}

func TestDailyIPQuotaZeroSkipsUnavailableUsageStorage(t *testing.T) {
	ctx := context.Background()
	repo := &panelRateLimitSettingRepo{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	quota, err := NewDailyIPTokenQuotaService(nil, svc)
	require.NoError(t, err)
	settings := DefaultDailyIPTokenQuotaSettings()
	settings.DailyTokenLimit = 0
	require.NoError(t, svc.SetDailyIPTokenQuotaSettings(ctx, settings))
	status, err := quota.Check(ctx, "203.0.113.8")
	require.NoError(t, err)
	require.Zero(t, status.Limit)
	list, err := quota.ListLimitedIPs(ctx, "", 1, 20)
	require.NoError(t, err)
	require.Empty(t, list.Items)
	require.Zero(t, list.Total)
}

func TestDailyIPQuotaWhitelistValidationNormalizationAndIsolation(t *testing.T) {
	for _, entries := range [][]string{
		{"not-an-ip"}, {"203.0.113.0/24"}, {"203.0.113.8:1234"}, {"fe80::1%en0"},
		{"203.0.113.8", "::ffff:203.0.113.8"}, {"2001:db8::1", "2001:0DB8:0:0::1"},
	} {
		settings := DefaultDailyIPTokenQuotaSettings()
		settings.Whitelist = entries
		require.Error(t, settings.Validate())
	}
	settings := DefaultDailyIPTokenQuotaSettings()
	settings.Whitelist = make([]string, MaxDailyIPWhitelistEntries+1)
	require.Error(t, settings.Validate())
	settings.Whitelist = nil
	for _, invalid := range []int64{-1, MaxDailyIPTokenLimit + 1} {
		settings.WhitelistDailyTokenLimit = invalid
		require.Error(t, settings.Validate())
	}
	settings.WhitelistDailyTokenLimit = 123
	ctx := context.Background()
	repo := &panelRateLimitSettingRepo{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	settings.Whitelist = []string{" ::ffff:203.0.113.8 ", "2001:0DB8::10"}
	require.NoError(t, svc.SetDailyIPTokenQuotaSettings(ctx, settings))
	settings.Whitelist[1] = "caller-mutated"
	got, err := svc.GetDailyIPTokenQuotaSettingsCached(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"203.0.113.8", "2001:db8::10"}, got.Whitelist)
	require.Equal(t, int64(123), got.WhitelistDailyTokenLimit)
	got.Whitelist[0] = "consumer-mutated"
	again, err := svc.GetDailyIPTokenQuotaSettingsCached(ctx)
	require.NoError(t, err)
	require.Equal(t, "203.0.113.8", again.Whitelist[0])
	restarted := NewSettingService(repo, &config.Config{})
	stored, err := restarted.GetDailyIPTokenQuotaSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, again, stored)
	for _, legacy := range []struct {
		value string
		limit int64
		ips   []string
	}{
		{`{"enabled":true,"daily_token_limit":100000000,"timezone":"Asia/Shanghai"}`, 0, []string{}},
		{`{"enabled":true,"daily_token_limit":100000000,"timezone":"Asia/Shanghai","whitelist":[{"ip_address":"203.0.113.8","daily_token_limit":100},{"ip_address":"203.0.113.9","daily_token_limit":100}]}`, 100, []string{"203.0.113.8", "203.0.113.9"}},
		{`{"enabled":true,"daily_token_limit":100000000,"timezone":"Asia/Shanghai","whitelist":[{"ip_address":"203.0.113.8","daily_token_limit":0},{"ip_address":"203.0.113.9","daily_token_limit":200}]}`, 200, []string{"203.0.113.8", "203.0.113.9"}},
	} {
		repo.values[SettingKeyDailyIPTokenQuota] = legacy.value
		stored, err = restarted.GetDailyIPTokenQuotaSettings(ctx)
		require.NoError(t, err)
		require.Equal(t, legacy.ips, stored.Whitelist)
		require.Equal(t, legacy.limit, stored.WhitelistDailyTokenLimit)
	}
}

func TestDailyIPQuotaWhitelistListingUsesEffectiveQuotas(t *testing.T) {
	ctx := context.Background()
	repo := &panelRateLimitSettingRepo{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	lister := &dailyIPListerStub{items: []DailyIPLimitedIP{{IPAddress: "203.0.113.8", UsedTokens: 100, DailyTokenLimit: 50, Whitelisted: true}}, total: 1}
	quota, err := NewDailyIPTokenQuotaService(lister, svc)
	require.NoError(t, err)
	settings := DefaultDailyIPTokenQuotaSettings()
	settings.DailyTokenLimit = 0
	settings.Whitelist = []string{"203.0.113.8", "203.0.113.9"}
	settings.WhitelistDailyTokenLimit = 50
	require.NoError(t, svc.SetDailyIPTokenQuotaSettings(ctx, settings))
	list, err := quota.ListLimitedIPs(ctx, "", 1, 20)
	require.NoError(t, err)
	require.Equal(t, settings.Whitelist, lister.whitelist)
	require.Equal(t, int64(50), lister.whitelistLimit)
	require.Zero(t, lister.limit)
	require.Equal(t, int64(50), list.Items[0].DailyTokenLimit)
	require.True(t, list.Items[0].Whitelisted)
	settings.WhitelistDailyTokenLimit = 0
	require.NoError(t, svc.SetDailyIPTokenQuotaSettings(ctx, settings))
	before := lister.calls
	list, err = quota.ListLimitedIPs(ctx, "", 1, 20)
	require.NoError(t, err)
	require.Empty(t, list.Items)
	require.Equal(t, before, lister.calls)
}
