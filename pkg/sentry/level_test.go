package sentry_test

import (
	"testing"

	sc "github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/zauberhaus/logger/pkg/logger"
	"github.com/zauberhaus/logger/pkg/sentry"
)

func TestLevel(t *testing.T) {
	tests := []struct {
		name  string
		level logger.Level
	}{
		{
			name:  "panic",
			level: logger.PanicLevel,
		},
		{
			name:  "fatal",
			level: logger.FatalLevel,
		},
		{
			name:  "error",
			level: logger.ErrorLevel,
		},
		{
			name:  "warn",
			level: logger.WarnLevel,
		},
		{
			name:  "info",
			level: logger.InfoLevel,
		},
		{
			name:  "debug",
			level: logger.DebugLevel,
		},
		{
			name:  "unknown",
			level: logger.Level(99),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txt := tt.level.String()
			switch txt {
			case "panic":
				txt = "fatal"
			case "warn":
				txt = "warning"
			case "Level(99)":
				txt = "info"
			}

			expected := sc.Level(txt)

			assert.Equal(t, expected, sentry.Level(tt.level))
		})
	}
}

func TestLogLevel(t *testing.T) {
	tests := []struct {
		name  string
		level sc.Level
	}{
		{
			name:  "fatal",
			level: sc.LevelFatal,
		},
		{
			name:  "error",
			level: sc.LevelError,
		},
		{
			name:  "warn",
			level: sc.LevelWarning,
		},
		{
			name:  "info",
			level: sc.LevelInfo,
		},
		{
			name:  "debug",
			level: sc.LevelDebug,
		},
		{
			name:  "unknown",
			level: sc.Level("unknown"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txt := string(tt.level)
			switch txt {
			case "warning":
				txt = "warn"
			case "unknown":
				txt = "info"
			}

			expected, err := logger.InfoLevel.Parse(txt)
			assert.NoError(t, err)

			assert.Equal(t, expected, sentry.LogLevel(tt.level))
		})
	}
}
