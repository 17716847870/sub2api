package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

const DefaultDailyIPTokenLimit int64 = 100_000_000

var ErrDailyIPTokenQuotaExceeded = errors.New("daily IP token quota exceeded")

// DailyIPTokenUsageRepository counts real tokens, independently of users, keys or prices.
type DailyIPTokenUsageRepository interface {
	GetTokenUsageByIP(ctx context.Context, clientIP string, start, end time.Time) (int64, error)
}

type DailyIPTokenQuotaStatus struct {
	Used    int64
	Limit   int64
	ResetAt time.Time
}

func (s DailyIPTokenQuotaStatus) Exhausted() bool { return s.Limit > 0 && s.Used >= s.Limit }

func (s DailyIPTokenQuotaStatus) Message() string {
	return fmt.Sprintf("This IP has reached its daily token limit (%d/%d). Access resumes at %s.", s.Used, s.Limit, s.ResetAt.Format(time.RFC3339))
}

type DailyIPTokenQuotaService struct {
	repo     DailyIPTokenUsageRepository
	settings *SettingService
	limit    int64
	location *time.Location
	now      func() time.Time
}

func NewDailyIPTokenQuotaService(repo DailyIPTokenUsageRepository, settings *SettingService) (*DailyIPTokenQuotaService, error) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nil, err
	}
	return &DailyIPTokenQuotaService{repo: repo, settings: settings, limit: DefaultDailyIPTokenLimit, location: location, now: time.Now}, nil
}

func (s *DailyIPTokenQuotaService) runtimeSettings(ctx context.Context) (DailyIPTokenQuotaSettings, error) {
	if s.settings != nil {
		return s.settings.GetDailyIPTokenQuotaSettingsCached(ctx)
	}
	return DailyIPTokenQuotaSettings{Enabled: s.limit > 0, DailyTokenLimit: s.limit, Timezone: s.location.String()}, nil
}

func (s *DailyIPTokenQuotaService) Enabled() bool {
	return s != nil && (s.settings != nil || s.limit > 0)
}

func (s *DailyIPTokenQuotaService) Check(ctx context.Context, clientIP string) (DailyIPTokenQuotaStatus, error) {
	if !s.Enabled() {
		return DailyIPTokenQuotaStatus{}, nil
	}
	settings, err := s.runtimeSettings(ctx)
	if err != nil {
		return DailyIPTokenQuotaStatus{}, err
	}
	if !settings.HasAnyLimit() {
		return DailyIPTokenQuotaStatus{}, nil
	}
	location, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		return DailyIPTokenQuotaStatus{}, err
	}
	address := net.ParseIP(clientIP)
	if address == nil {
		return DailyIPTokenQuotaStatus{}, errors.New("unable to determine client IP for daily token quota")
	}
	now := s.now().In(location)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 1)
	limit, _ := settings.LimitForIP(address.String())
	status := DailyIPTokenQuotaStatus{Limit: limit, ResetAt: end}
	if limit == 0 {
		return status, nil
	}
	if s.repo == nil {
		return status, errors.New("daily IP token quota repository is unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	used, err := s.repo.GetTokenUsageByIP(ctx, address.String(), start, end)
	if err != nil {
		return status, fmt.Errorf("read daily IP token usage: %w", err)
	}
	status.Used = used
	if status.Exhausted() {
		return status, ErrDailyIPTokenQuotaExceeded
	}
	return status, nil
}

type dailyIPTokenQuotaContextKey struct{}
type dailyIPTokenQuotaBinding struct {
	service *DailyIPTokenQuotaService
	ip      string
}

// WithDailyIPTokenQuota also lets a long-lived Responses WebSocket recheck each turn.
func WithDailyIPTokenQuota(ctx context.Context, s *DailyIPTokenQuotaService, clientIP string) context.Context {
	return context.WithValue(ctx, dailyIPTokenQuotaContextKey{}, dailyIPTokenQuotaBinding{service: s, ip: clientIP})
}

func CheckDailyIPTokenQuota(ctx context.Context) (DailyIPTokenQuotaStatus, error) {
	binding, ok := ctx.Value(dailyIPTokenQuotaContextKey{}).(dailyIPTokenQuotaBinding)
	if !ok {
		return DailyIPTokenQuotaStatus{}, nil
	}
	return binding.service.Check(ctx, binding.ip)
}

// DailyIPTokenQuotaEnabled makes quota-sensitive usage accounting synchronous:
// a sampled/dropped asynchronous billing task must not erase consumed tokens.
func DailyIPTokenQuotaEnabled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	binding, ok := ctx.Value(dailyIPTokenQuotaContextKey{}).(dailyIPTokenQuotaBinding)
	return ok && binding.service.Enabled()
}

// DailyIPTokenUsageLister is separated from the hot-path repository to keep admission focused.
type DailyIPTokenUsageLister interface {
	ListLimitedIPs(ctx context.Context, start, end time.Time, limit, whitelistLimit int64, whitelist []string, search string, page, pageSize int) ([]DailyIPLimitedIP, int64, error)
}

type DailyIPLimitedIP struct {
	IPAddress       string    `json:"ip_address"`
	DailyTokenLimit int64     `json:"daily_token_limit"`
	Whitelisted     bool      `json:"whitelisted"`
	UsedTokens      int64     `json:"used_tokens"`
	RequestCount    int64     `json:"request_count"`
	LastUsedAt      time.Time `json:"last_used_at"`
	ResetAt         time.Time `json:"reset_at"`
}

type DailyIPLimitedIPList struct {
	Items      []DailyIPLimitedIP        `json:"items"`
	Total      int64                     `json:"total"`
	Page       int                       `json:"page"`
	PageSize   int                       `json:"page_size"`
	Settings   DailyIPTokenQuotaSettings `json:"settings"`
	DayStart   time.Time                 `json:"day_start"`
	ResetAt    time.Time                 `json:"reset_at"`
	ServerTime time.Time                 `json:"server_time"`
}

func (s *DailyIPTokenQuotaService) ListLimitedIPs(ctx context.Context, search string, page, pageSize int) (*DailyIPLimitedIPList, error) {
	if s == nil {
		return nil, errors.New("daily IP quota service unavailable")
	}
	if page < 1 || pageSize < 1 || pageSize > 100 || page > 1_000_000 {
		return nil, errors.New("invalid pagination")
	}
	settings, err := s.runtimeSettings(ctx)
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		return nil, err
	}
	now := s.now().In(location)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	end := start.AddDate(0, 0, 1)
	result := &DailyIPLimitedIPList{Items: []DailyIPLimitedIP{}, Page: page, PageSize: pageSize, Settings: settings, DayStart: start, ResetAt: end, ServerTime: now}
	if !settings.HasAnyLimit() {
		return result, nil
	}
	lister, ok := s.repo.(DailyIPTokenUsageLister)
	if !ok {
		return nil, errors.New("daily IP quota listing unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	items, total, err := lister.ListLimitedIPs(ctx, start, end, settings.DailyTokenLimit, settings.WhitelistDailyTokenLimit, settings.Whitelist, search, page, pageSize)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].ResetAt = end
	}
	if items != nil {
		result.Items = items
	}
	result.Total = total
	return result, nil
}
