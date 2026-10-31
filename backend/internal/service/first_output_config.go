package service

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	FirstOutputLimitSecondsKey    = "temp_unschedulable_first_output_limit_seconds"
	FirstOutputCooldownMinutesKey = "temp_unschedulable_first_output_cooldown_minutes"
	FirstOutputWindowMinutesKey   = "temp_unschedulable_first_output_window_minutes"
	FirstOutputMaxLimitSeconds    = 3600
	FirstOutputMaxMinutes         = 1440
)

// FirstOutputProtectionConfig is a value snapshot shared by the observer,
// counter and conditional database write for one upstream request.
type FirstOutputProtectionConfig struct {
	LimitSeconds    int
	CooldownMinutes int
	WindowMinutes   int
}

func DefaultFirstOutputProtectionConfig() FirstOutputProtectionConfig {
	return FirstOutputProtectionConfig{int(FirstOutputLimit / time.Second), int(FirstOutputCooldown / time.Minute), int(FirstOutputWindow / time.Minute)}
}

func firstOutputPositiveInteger(value any, maximum int) (int, bool) {
	var number float64
	switch value := value.(type) {
	case int:
		number = float64(value)
	case int64:
		number = float64(value)
	case float64:
		number = value
	case json.Number:
		var err error
		number, err = value.Float64()
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	if math.IsNaN(number) || math.IsInf(number, 0) || number < 1 || number > float64(maximum) || math.Trunc(number) != number {
		return 0, false
	}
	return int(number), true
}

// NormalizeFirstOutputProtectionCredentials validates only provided fields so
// bulk credentials patches preserve the account's other settings.
func NormalizeFirstOutputProtectionCredentials(credentials map[string]any) error {
	for _, field := range []struct {
		key     string
		maximum int
	}{
		{FirstOutputLimitSecondsKey, FirstOutputMaxLimitSeconds},
		{FirstOutputCooldownMinutesKey, FirstOutputMaxMinutes},
		{FirstOutputWindowMinutesKey, FirstOutputMaxMinutes},
	} {
		value, exists := credentials[field.key]
		if !exists {
			continue
		}
		parsed, valid := firstOutputPositiveInteger(value, field.maximum)
		if !valid {
			return infraerrors.BadRequest("FIRST_OUTPUT_CONFIG_INVALID", fmt.Sprintf("%s must be an integer between 1 and %d", field.key, field.maximum))
		}
		credentials[field.key] = parsed
	}
	return nil
}

func (a *Account) FirstOutputProtectionConfig() FirstOutputProtectionConfig {
	config := DefaultFirstOutputProtectionConfig()
	if a == nil {
		return config
	}
	if value, ok := firstOutputPositiveInteger(a.Credentials[FirstOutputLimitSecondsKey], FirstOutputMaxLimitSeconds); ok {
		config.LimitSeconds = value
	}
	if value, ok := firstOutputPositiveInteger(a.Credentials[FirstOutputCooldownMinutesKey], FirstOutputMaxMinutes); ok {
		config.CooldownMinutes = value
	}
	if value, ok := firstOutputPositiveInteger(a.Credentials[FirstOutputWindowMinutesKey], FirstOutputMaxMinutes); ok {
		config.WindowMinutes = value
	}
	return config
}

func (c FirstOutputProtectionConfig) limit() time.Duration {
	return time.Duration(c.LimitSeconds) * time.Second
}
func (c FirstOutputProtectionConfig) cooldown() time.Duration {
	return time.Duration(c.CooldownMinutes) * time.Minute
}
func (c FirstOutputProtectionConfig) window() time.Duration {
	return time.Duration(c.WindowMinutes) * time.Minute
}

func firstOutputCounterScope(generation string, config FirstOutputProtectionConfig) string {
	// Preserve existing default windows across upgrades and treat omitted and
	// explicitly saved defaults identically. Other policies count independently.
	if config == DefaultFirstOutputProtectionConfig() {
		return generation
	}
	return fmt.Sprintf("%s|%d:%d:%d", generation, config.LimitSeconds, config.CooldownMinutes, config.WindowMinutes)
}
