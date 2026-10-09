package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type dailyIPTokenUsageStub struct {
	used       int64
	err        error
	calls      int
	ip         string
	start, end time.Time
}

func (r *dailyIPTokenUsageStub) GetTokenUsageByIP(_ context.Context, ip string, start, end time.Time) (int64, error) {
	r.calls++
	r.ip, r.start, r.end = ip, start, end
	return r.used, r.err
}

func TestDailyIPTokenQuotaBoundaries(t *testing.T) {
	for _, used := range []int64{0, DefaultDailyIPTokenLimit - 1, DefaultDailyIPTokenLimit, DefaultDailyIPTokenLimit + 1, 3_000_000_000} {
		t.Run(time.Duration(used).String(), func(t *testing.T) {
			repo := &dailyIPTokenUsageStub{used: used}
			svc, err := NewDailyIPTokenQuotaService(repo, nil)
			require.NoError(t, err)
			status, err := svc.Check(context.Background(), "203.0.113.8")
			require.Equal(t, used, status.Used)
			require.Equal(t, DefaultDailyIPTokenLimit, status.Limit)
			if used >= DefaultDailyIPTokenLimit {
				require.ErrorIs(t, err, ErrDailyIPTokenQuotaExceeded)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDailyIPTokenQuotaMidnight(t *testing.T) {
	repo := &dailyIPTokenUsageStub{used: DefaultDailyIPTokenLimit}
	svc, err := NewDailyIPTokenQuotaService(repo, nil)
	require.NoError(t, err)
	before := time.Date(2026, 10, 8, 15, 59, 59, 0, time.UTC)
	svc.now = func() time.Time { return before }
	status, err := svc.Check(context.Background(), "203.0.113.8")
	require.ErrorIs(t, err, ErrDailyIPTokenQuotaExceeded)
	require.Equal(t, "2026-10-08T00:00:00+08:00", repo.start.Format(time.RFC3339))
	require.Equal(t, "2026-10-09T00:00:00+08:00", status.ResetAt.Format(time.RFC3339))
	// A different half-open day range automatically restores admission, no cron job.
	svc.now = func() time.Time { return before.Add(time.Second) }
	repo.used = 0
	status, err = svc.Check(context.Background(), "203.0.113.8")
	require.NoError(t, err)
	require.Equal(t, "2026-10-09T00:00:00+08:00", repo.start.Format(time.RFC3339))
	require.Equal(t, "2026-10-10T00:00:00+08:00", status.ResetAt.Format(time.RFC3339))
}

func TestDailyIPTokenQuotaDSTDay(t *testing.T) {
	repo := &dailyIPTokenUsageStub{}
	svc, err := NewDailyIPTokenQuotaService(repo, nil)
	svc.limit = 100
	svc.location, _ = time.LoadLocation("America/New_York")
	require.NoError(t, err)
	svc.now = func() time.Time { return time.Date(2026, 3, 8, 18, 0, 0, 0, time.UTC) }
	_, err = svc.Check(context.Background(), "2001:db8::1")
	require.NoError(t, err)
	require.Equal(t, 23*time.Hour, repo.end.Sub(repo.start))
}

func TestDailyIPTokenQuotaDisabledAndFailures(t *testing.T) {
	repo := &dailyIPTokenUsageStub{err: errors.New("database unavailable")}
	disabled, err := NewDailyIPTokenQuotaService(repo, nil)
	disabled.limit = 0
	require.NoError(t, err)
	_, err = disabled.Check(context.Background(), "invalid")
	require.NoError(t, err)
	require.Zero(t, repo.calls)
	enabled, err := NewDailyIPTokenQuotaService(repo, nil)
	require.NoError(t, err)
	_, err = enabled.Check(context.Background(), "invalid")
	require.Error(t, err)
	require.Zero(t, repo.calls)
	_, err = enabled.Check(context.Background(), "::ffff:203.0.113.8")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrDailyIPTokenQuotaExceeded)
	require.Equal(t, "203.0.113.8", repo.ip)
}

func TestDailyIPTokenQuotaWebSocketContextRechecks(t *testing.T) {
	repo := &dailyIPTokenUsageStub{}
	svc, err := NewDailyIPTokenQuotaService(repo, nil)
	require.NoError(t, err)
	ctx := WithDailyIPTokenQuota(context.Background(), svc, "203.0.113.8")
	_, err = CheckDailyIPTokenQuota(ctx)
	require.NoError(t, err)
	repo.used = DefaultDailyIPTokenLimit
	_, err = CheckDailyIPTokenQuota(ctx)
	require.ErrorIs(t, err, ErrDailyIPTokenQuotaExceeded)
	require.Equal(t, 2, repo.calls)
	_, err = CheckDailyIPTokenQuota(context.Background())
	require.NoError(t, err)
}
