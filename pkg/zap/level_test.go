package zap_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zauberhaus/logger/pkg/logger"
	"github.com/zauberhaus/logger/pkg/zap"
	"go.uber.org/zap/zapcore"
)

func TestLevel(t *testing.T) {
	tests := []struct {
		name  string
		level logger.Level
	}{
		{
			name:  "fatal",
			level: logger.FatalLevel,
		},
		{
			name:  "panic",
			level: logger.PanicLevel,
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
			expected, err := zapcore.ParseLevel(tt.level.String())
			if tt.name == "unknown" {
				assert.Error(t, err)
				expected = zapcore.InfoLevel
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, expected, zap.Level(tt.level))
		})
	}
}

func TestLogLevel(t *testing.T) {
	tests := []struct {
		name  string
		level zapcore.Level
	}{
		{
			name:  "fatal",
			level: zapcore.FatalLevel,
		},
		{
			name:  "panic",
			level: zapcore.PanicLevel,
		},
		{
			name:  "error",
			level: zapcore.ErrorLevel,
		},
		{
			name:  "warn",
			level: zapcore.WarnLevel,
		},
		{
			name:  "info",
			level: zapcore.InfoLevel,
		},
		{
			name:  "debug",
			level: zapcore.DebugLevel,
		},
		{
			name:  "unknown",
			level: zapcore.Level(99),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected, err := logger.InfoLevel.Parse(tt.level.String())
			if tt.name == "unknown" {
				assert.Error(t, err)
				expected = logger.InfoLevel
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, expected, zap.LogLevel(tt.level))
		})
	}
}
