package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type dailyIPTokenUsageRepository struct{ db *sql.DB }

func NewDailyIPTokenUsageRepository(db *sql.DB) service.DailyIPTokenUsageRepository {
	return &dailyIPTokenUsageRepository{db: db}
}

// Match UsageLog.TotalTokens: cached tokens count once, without pricing multipliers.
// Casting each component BEFORE addition prevents PostgreSQL integer overflow.
const dailyIPTokenUsageQuery = `
SELECT COALESCE(SUM(
    GREATEST(input_tokens::bigint, 0) + GREATEST(output_tokens::bigint, 0) +
    GREATEST(cache_creation_tokens::bigint, 0) + GREATEST(cache_read_tokens::bigint, 0)
), 0)::bigint
FROM usage_logs
WHERE ip_address = $1 AND created_at >= $2 AND created_at < $3`

func (r *dailyIPTokenUsageRepository) GetTokenUsageByIP(ctx context.Context, clientIP string, start, end time.Time) (int64, error) {
	var used int64
	err := r.db.QueryRowContext(ctx, dailyIPTokenUsageQuery, clientIP, start, end).Scan(&used)
	return used, err
}

const limitedIPTokenUsageCTE = `WITH limited AS (
    SELECT ip_address,
        SUM(GREATEST(input_tokens::bigint, 0) + GREATEST(output_tokens::bigint, 0) +
            GREATEST(cache_creation_tokens::bigint, 0) + GREATEST(cache_read_tokens::bigint, 0))::bigint AS used_tokens,
        COUNT(*) AS request_count, MAX(created_at) AS last_used_at
    FROM usage_logs
    WHERE created_at >= $1 AND created_at < $2 AND ip_address IS NOT NULL AND ip_address <> ''
        AND ($4 = '' OR strpos(ip_address, $4) > 0)
    GROUP BY ip_address
    HAVING SUM(GREATEST(input_tokens::bigint, 0) + GREATEST(output_tokens::bigint, 0) +
        GREATEST(cache_creation_tokens::bigint, 0) + GREATEST(cache_read_tokens::bigint, 0)) >= $3
)`

func (r *dailyIPTokenUsageRepository) ListLimitedIPs(ctx context.Context, start, end time.Time, limit int64, search string, page, pageSize int) ([]service.DailyIPLimitedIP, int64, error) {
	// Both queries see the same snapshot, even as generation usage is being written.
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	args := []any{start, end, limit, search}
	var total int64
	if err := tx.QueryRowContext(ctx, limitedIPTokenUsageCTE+` SELECT COUNT(*) FROM limited`, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := tx.QueryContext(ctx, limitedIPTokenUsageCTE+` SELECT ip_address, used_tokens, request_count, last_used_at FROM limited ORDER BY used_tokens DESC, ip_address ASC LIMIT $5 OFFSET $6`, append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []service.DailyIPLimitedIP{}
	for rows.Next() {
		var item service.DailyIPLimitedIP
		if err := rows.Scan(&item.IPAddress, &item.UsedTokens, &item.RequestCount, &item.LastUsedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if err := rows.Close(); err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
