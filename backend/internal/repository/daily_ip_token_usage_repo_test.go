package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestDailyIPTokenUsageRepository(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewDailyIPTokenUsageRepository(db)
	start := time.Date(2026, 10, 8, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	end := start.AddDate(0, 0, 1)
	for _, used := range []int64{0, 100_000_000, 3_000_000_000} {
		mock.ExpectQuery(regexp.QuoteMeta(dailyIPTokenUsageQuery)).WithArgs("203.0.113.8", start, end).WillReturnRows(sqlmock.NewRows([]string{"tokens"}).AddRow(used))
		got, err := repo.GetTokenUsageByIP(context.Background(), "203.0.113.8", start, end)
		require.NoError(t, err)
		require.Equal(t, used, got)
	}
	mock.ExpectQuery(regexp.QuoteMeta(dailyIPTokenUsageQuery)).WithArgs("203.0.113.8", start, end).WillReturnError(errors.New("offline"))
	_, err = repo.GetTokenUsageByIP(context.Background(), "203.0.113.8", start, end)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDailyIPTokenUsageListPaginationSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := NewDailyIPTokenUsageRepository(db).(interface {
		ListLimitedIPs(context.Context, time.Time, time.Time, int64, int64, []string, string, int, int) ([]service.DailyIPLimitedIP, int64, error)
	})
	start := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(limitedIPTokenUsageCTE+` SELECT COUNT(*) FROM limited`)).WithArgs(start, end, int64(100000000), "203.0", int64(0), "[]").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(regexp.QuoteMeta(limitedIPTokenUsageCTE+` SELECT ip_address, used_tokens, request_count, last_used_at, daily_token_limit, whitelisted FROM limited ORDER BY used_tokens DESC, ip_address ASC LIMIT $7 OFFSET $8`)).WithArgs(start, end, int64(100000000), "203.0", int64(0), "[]", 20, 20).WillReturnRows(sqlmock.NewRows([]string{"ip_address", "used_tokens", "request_count", "last_used_at", "daily_token_limit", "whitelisted"}))
	mock.ExpectCommit()
	items, total, err := repo.ListLimitedIPs(context.Background(), start, end, 100000000, 0, nil, "203.0", 2, 20)
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, int64(2), total, "empty later pages must retain the true total")
	require.NoError(t, mock.ExpectationsWereMet())
}
