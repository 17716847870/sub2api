package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/textproto"
	"strings"
	"time"

	"golang.org/x/net/http/httpguts"
)

const SettingKeyGatewayHeaderAuth = "gateway_header_authentication"
const gatewayHeaderAuthSettingsCacheTTL = 5 * time.Second

const MaxGatewayHeaderAuthRules = 32

type GatewayHeaderAuthRule struct {
	HeaderName        string `json:"header_name"`
	RequiredSubstring string `json:"required_substring"`
}

type GatewayHeaderAuthSettings struct {
	Enabled bool                    `json:"enabled"`
	Rules   []GatewayHeaderAuthRule `json:"rules"`
}

type cachedGatewayHeaderAuthSettings struct {
	settings  GatewayHeaderAuthSettings
	expiresAt time.Time
}

func DefaultGatewayHeaderAuthSettings() GatewayHeaderAuthSettings {
	return GatewayHeaderAuthSettings{Enabled: true, Rules: []GatewayHeaderAuthRule{{HeaderName: "User-Agent", RequiredSubstring: "XundaAI"}}}
}

func (v GatewayHeaderAuthRule) Validate() error {
	if len(v.HeaderName) > 128 || !httpguts.ValidHeaderFieldName(v.HeaderName) {
		return errors.New("header_name must be a valid HTTP header name (maximum 128 bytes)")
	}
	if strings.TrimSpace(v.RequiredSubstring) == "" || len(v.RequiredSubstring) > 512 || !httpguts.ValidHeaderFieldValue(v.RequiredSubstring) {
		return errors.New("required_substring must be non-empty valid header text (maximum 512 bytes)")
	}
	return nil
}

func (v GatewayHeaderAuthSettings) Validate() error {
	if v.Enabled && len(v.Rules) == 0 {
		return errors.New("at least one header rule is required when authentication is enabled")
	}
	if len(v.Rules) > MaxGatewayHeaderAuthRules {
		return fmt.Errorf("at most %d header rules are allowed", MaxGatewayHeaderAuthRules)
	}
	for i, rule := range v.Rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("rule %d: %w", i+1, err)
		}
	}
	return nil
}

// Copy the rule slice so callers cannot mutate the hot-path cache after reading
// settings or passing their input to the setter.
func cloneGatewayHeaderAuthSettings(settings GatewayHeaderAuthSettings) GatewayHeaderAuthSettings {
	rules := make([]GatewayHeaderAuthRule, len(settings.Rules))
	copy(rules, settings.Rules)
	settings.Rules = rules
	return settings
}

// Existing single-header records are interpreted as one rule without discarding
// the administrator's configured values or enabled state.
func decodeGatewayHeaderAuthSettings(value string) (GatewayHeaderAuthSettings, error) {
	var stored struct {
		Enabled           *bool           `json:"enabled"`
		Rules             json.RawMessage `json:"rules"`
		HeaderName        *string         `json:"header_name"`
		RequiredSubstring *string         `json:"required_substring"`
	}
	if err := json.Unmarshal([]byte(value), &stored); err != nil {
		return GatewayHeaderAuthSettings{}, err
	}
	settings := GatewayHeaderAuthSettings{Enabled: true}
	if stored.Enabled != nil {
		settings.Enabled = *stored.Enabled
	}
	if len(stored.Rules) > 0 {
		if string(stored.Rules) == "null" {
			return settings, errors.New("rules must be an array")
		}
		if err := json.Unmarshal(stored.Rules, &settings.Rules); err != nil {
			return settings, err
		}
	} else if stored.HeaderName != nil && stored.RequiredSubstring != nil {
		settings.Rules = []GatewayHeaderAuthRule{{HeaderName: *stored.HeaderName, RequiredSubstring: *stored.RequiredSubstring}}
	} else {
		return settings, errors.New("header authentication rules are missing")
	}
	if err := settings.Validate(); err != nil {
		return settings, err
	}
	for i := range settings.Rules {
		settings.Rules[i].HeaderName = textproto.CanonicalMIMEHeaderKey(settings.Rules[i].HeaderName)
	}
	return settings, nil
}

func (s *SettingService) GetGatewayHeaderAuthSettings(ctx context.Context) (GatewayHeaderAuthSettings, error) {
	settings := DefaultGatewayHeaderAuthSettings()
	value, err := s.settingRepo.GetValue(ctx, SettingKeyGatewayHeaderAuth)
	if errors.Is(err, ErrSettingNotFound) {
		return settings, nil
	}
	if err != nil {
		return settings, fmt.Errorf("read gateway header authentication settings: %w", err)
	}
	if strings.TrimSpace(value) == "" {
		return settings, nil
	}
	settings, err = decodeGatewayHeaderAuthSettings(value)
	if err != nil {
		return settings, fmt.Errorf("invalid gateway header authentication settings: %w", err)
	}
	return settings, nil
}

func (s *SettingService) GetGatewayHeaderAuthSettingsCached(ctx context.Context) (GatewayHeaderAuthSettings, error) {
	// A refresh cannot overwrite a setting saved concurrently on this node.
	s.gatewayHeaderAuthSettingsMu.Lock()
	defer s.gatewayHeaderAuthSettingsMu.Unlock()
	if cached := s.gatewayHeaderAuthSettingsCache; cached != nil && time.Now().Before(cached.expiresAt) {
		return cloneGatewayHeaderAuthSettings(cached.settings), nil
	}
	readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	settings, err := s.GetGatewayHeaderAuthSettings(readCtx)
	if err != nil {
		return settings, err
	}
	s.gatewayHeaderAuthSettingsCache = &cachedGatewayHeaderAuthSettings{settings: cloneGatewayHeaderAuthSettings(settings), expiresAt: time.Now().Add(gatewayHeaderAuthSettingsCacheTTL)}
	return settings, nil
}

func (s *SettingService) SetGatewayHeaderAuthSettings(ctx context.Context, settings GatewayHeaderAuthSettings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	settings = cloneGatewayHeaderAuthSettings(settings)
	for i := range settings.Rules {
		settings.Rules[i].HeaderName = textproto.CanonicalMIMEHeaderKey(settings.Rules[i].HeaderName)
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	s.gatewayHeaderAuthSettingsMu.Lock()
	defer s.gatewayHeaderAuthSettingsMu.Unlock()
	if err := s.settingRepo.Set(ctx, SettingKeyGatewayHeaderAuth, string(data)); err != nil {
		return err
	}
	s.gatewayHeaderAuthSettingsCache = &cachedGatewayHeaderAuthSettings{settings: cloneGatewayHeaderAuthSettings(settings), expiresAt: time.Now().Add(gatewayHeaderAuthSettingsCacheTTL)}
	return nil
}
