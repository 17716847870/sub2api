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
		{Enabled: true, DailyTokenLimit: 0, Timezone: "Asia/Shanghai"},
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
	items      []DailyIPLimitedIP
	total      int64
	limit      int64
	start, end time.Time
}

func (r *dailyIPListerStub) GetTokenUsageByIP(context.Context, string, time.Time, time.Time) (int64, error) {
	return 0, nil
}
func (r *dailyIPListerStub) ListLimitedIPs(_ context.Context, start, end time.Time, limit int64, _ string, _, _ int) ([]DailyIPLimitedIP, int64, error) {
	r.limit, r.start, r.end = limit, start, end
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
