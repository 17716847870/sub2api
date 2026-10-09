package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGatewayHeaderAuthSettingsDefaultsPersistenceAndHotUpdate(t *testing.T) {
	repo := &panelRateLimitSettingRepo{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	ctx := context.Background()
	got, err := svc.GetGatewayHeaderAuthSettingsCached(ctx)
	require.NoError(t, err)
	require.Equal(t, DefaultGatewayHeaderAuthSettings(), got)
	updated := GatewayHeaderAuthSettings{Enabled: true, Rules: []GatewayHeaderAuthRule{{HeaderName: "x-client-auth", RequiredSubstring: "client-allowed"}}}
	require.NoError(t, svc.SetGatewayHeaderAuthSettings(ctx, updated))
	got, err = svc.GetGatewayHeaderAuthSettingsCached(ctx)
	require.NoError(t, err)
	require.Equal(t, "X-Client-Auth", got.Rules[0].HeaderName)
	require.Equal(t, updated.Rules[0].RequiredSubstring, got.Rules[0].RequiredSubstring)
	calls := repo.getValueCalls
	_, err = svc.GetGatewayHeaderAuthSettingsCached(ctx)
	require.NoError(t, err)
	require.Equal(t, calls, repo.getValueCalls, "fresh config must not query DB for each request")
	restarted := NewSettingService(repo, &config.Config{})
	stored, err := restarted.GetGatewayHeaderAuthSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, got, stored)
	updated.Enabled = false
	require.NoError(t, svc.SetGatewayHeaderAuthSettings(ctx, updated))
	got, err = svc.GetGatewayHeaderAuthSettingsCached(ctx)
	require.NoError(t, err)
	require.False(t, got.Enabled, "disable must apply immediately")
}

func TestGatewayHeaderAuthSettingsValidation(t *testing.T) {
	for _, header := range []string{"", "User Agent", "User-Agent:", "X-Auth\r\n", strings.Repeat("a", 129)} {
		settings := DefaultGatewayHeaderAuthSettings()
		settings.Rules[0].HeaderName = header
		require.Error(t, settings.Validate(), header)
	}
	for _, value := range []string{"", "  ", "\r\nInjected: header", "a\x00b", strings.Repeat("a", 513)} {
		settings := DefaultGatewayHeaderAuthSettings()
		settings.Rules[0].RequiredSubstring = value
		require.Error(t, settings.Validate())
	}
	for _, value := range []string{"XundaAI", "XundaAI/1.0", "app-允许", "bearer token"} {
		settings := DefaultGatewayHeaderAuthSettings()
		settings.Rules[0].RequiredSubstring = value
		require.NoError(t, settings.Validate())
	}
}

func TestGatewayHeaderAuthSettingsFailClosed(t *testing.T) {
	repo := &panelRateLimitSettingRepo{values: map[string]string{SettingKeyGatewayHeaderAuth: "broken-json"}}
	svc := NewSettingService(repo, &config.Config{})
	_, err := svc.GetGatewayHeaderAuthSettingsCached(context.Background())
	require.Error(t, err)
	repo.values[SettingKeyGatewayHeaderAuth] = `{"enabled":true,"header_name":"User-Agent","required_substring":""}`
	_, err = svc.GetGatewayHeaderAuthSettingsCached(context.Background())
	require.Error(t, err)
	repo.values[SettingKeyGatewayHeaderAuth] = `{"enabled":true,"header_name":"User-Agent","required_substring":"XundaAI"}`
	_, err = svc.GetGatewayHeaderAuthSettingsCached(context.Background())
	require.NoError(t, err)
	svc.gatewayHeaderAuthSettingsCache.expiresAt = time.Time{}
	repo.getValueErr = errors.New("settings offline")
	_, err = svc.GetGatewayHeaderAuthSettingsCached(context.Background())
	require.ErrorContains(t, err, "settings offline")
}

func TestGatewayHeaderAuthLegacySingleRulePreserved(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		value := fmt.Sprintf(`{"enabled":%t,"header_name":"x-original-client","required_substring":"keep-this"}`, enabled)
		repo := &panelRateLimitSettingRepo{values: map[string]string{SettingKeyGatewayHeaderAuth: value}}
		svc := NewSettingService(repo, &config.Config{})
		got, err := svc.GetGatewayHeaderAuthSettings(context.Background())
		require.NoError(t, err)
		require.Equal(t, enabled, got.Enabled)
		require.Equal(t, []GatewayHeaderAuthRule{{HeaderName: "X-Original-Client", RequiredSubstring: "keep-this"}}, got.Rules)
		require.NoError(t, svc.SetGatewayHeaderAuthSettings(context.Background(), got))
		require.Contains(t, repo.values[SettingKeyGatewayHeaderAuth], `"rules":[`)
		restarted := NewSettingService(repo, &config.Config{})
		stored, err := restarted.GetGatewayHeaderAuthSettings(context.Background())
		require.NoError(t, err)
		require.Equal(t, got, stored)
	}
}

func TestGatewayHeaderAuthRulesValidationAndCacheIsolation(t *testing.T) {
	ctx := context.Background()
	repo := &panelRateLimitSettingRepo{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	settings := DefaultGatewayHeaderAuthSettings()
	settings.Rules = append(settings.Rules, GatewayHeaderAuthRule{HeaderName: "x-client-auth", RequiredSubstring: "app"})
	require.NoError(t, svc.SetGatewayHeaderAuthSettings(ctx, settings))
	settings.Rules[1].RequiredSubstring = "caller-mutated"
	got, err := svc.GetGatewayHeaderAuthSettingsCached(ctx)
	require.NoError(t, err)
	require.Equal(t, "app", got.Rules[1].RequiredSubstring)
	got.Rules[0].RequiredSubstring = "consumer-mutated"
	second, err := svc.GetGatewayHeaderAuthSettingsCached(ctx)
	require.NoError(t, err)
	require.Equal(t, "XundaAI", second.Rules[0].RequiredSubstring)
	require.Equal(t, "X-Client-Auth", second.Rules[1].HeaderName)
	require.Error(t, (GatewayHeaderAuthSettings{Enabled: true}).Validate())
	require.NoError(t, (GatewayHeaderAuthSettings{Enabled: false}).Validate())
	tooMany := GatewayHeaderAuthSettings{Enabled: true}
	for i := 0; i <= MaxGatewayHeaderAuthRules; i++ {
		tooMany.Rules = append(tooMany.Rules, DefaultGatewayHeaderAuthSettings().Rules[0])
	}
	require.Error(t, tooMany.Validate())
	for _, value := range []string{
		`{"enabled":true,"rules":[]}`, `{"enabled":true,"rules":null}`,
		`{"enabled":true,"rules":[null]}`, `{"enabled":true,"rules":[{}]}`,
		`{"enabled":true}`, `{"enabled":true,"rules":{},"header_name":"User-Agent","required_substring":"XundaAI"}`,
	} {
		_, err := decodeGatewayHeaderAuthSettings(value)
		require.Error(t, err, value)
	}
}
