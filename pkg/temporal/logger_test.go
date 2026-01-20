package temporal_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zauberhaus/logger/pkg/logger"
	"github.com/zauberhaus/logger/pkg/memory"
	"github.com/zauberhaus/logger/pkg/mock"
	"github.com/zauberhaus/logger/pkg/temporal"
	"github.com/zauberhaus/logger/pkg/zap"
	"go.uber.org/mock/gomock"
)

func TestNewTemporalLogger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockLogger := mock.NewMockLogger(ctrl)

	l := temporal.NewLogger(mockLogger)
	if l == nil {
		t.Error("Expected non-nil, got nil")
	}
}

func TestNewTemporalMemoryLogger(t *testing.T) {
	type message struct {
		Level      logger.Level `json:"level"`
		Message    string       `json:"msg"`
		Time       time.Time    `json:"ts"`
		Caller     string       `json:"caller"`
		Stacktrace string       `json:"stacktrace"`
		Keys       []any        `json:"keys"`
	}

	m := memory.NewLogger(memory.WithBlocking(true), zap.WithOutput(zap.JSONOutput), zap.Skip(2))
	l := temporal.NewLogger(m)
	assert.NotNil(t, l)

	l.Info("test message", "abc", 1)
	l.Error("test error", 99)

	txt := m.NextLine()

	var msg message
	decoder := json.NewDecoder(bytes.NewBuffer([]byte(txt)))
	decoder.DisallowUnknownFields()

	err := decoder.Decode(&msg)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, msg.Time)
		assert.Equal(t, msg.Level, logger.InfoLevel)
		assert.Equal(t, msg.Message, "test message")
		assert.Empty(t, msg.Stacktrace)
		assert.Contains(t, msg.Caller, "temporal/logger_test.go:")
		assert.Equal(t, []any{"abc", float64(1)}, msg.Keys)
	}

	txt = m.NextLine()

	decoder = json.NewDecoder(bytes.NewBuffer([]byte(txt)))
	decoder.DisallowUnknownFields()

	err = decoder.Decode(&msg)
	if assert.NoError(t, err) {
		assert.NotEmpty(t, msg.Time)
		assert.Equal(t, msg.Level, logger.ErrorLevel)
		assert.Equal(t, msg.Message, "test error")
		assert.NotEmpty(t, msg.Stacktrace)
		assert.Contains(t, msg.Caller, "temporal/logger_test.go:")
		assert.Equal(t, []any{float64(99)}, msg.Keys)
	}

	assert.Equal(t, 0, m.Len())
}

func TestTemporalLogger_Levels(t *testing.T) {
	testCases := []struct {
		name        string
		logFunc     func(l logger.Logger, msg string, keyvals ...any)
		levelMethod string
	}{
		{
			"Debug",
			func(l logger.Logger, msg string, keyvals ...any) {
				temporal.NewLogger(l).Debug(msg, keyvals...)
			},
			"Debug",
		},
		{
			"Info",
			func(l logger.Logger, msg string, keyvals ...any) {
				temporal.NewLogger(l).Info(msg, keyvals...)
			},
			"Info",
		},
		{
			"Warn",
			func(l logger.Logger, msg string, keyvals ...any) {
				temporal.NewLogger(l).Warn(msg, keyvals...)
			},
			"Warn",
		},
		{
			"Error",
			func(l logger.Logger, msg string, keyvals ...any) {
				temporal.NewLogger(l).Error(msg, keyvals...)
			},
			"Error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockLogger := mock.NewMockLogger(ctrl)
			withLogger := mock.NewMockLogger(ctrl)

			keyvals := []any{"keys", []any{"key", "value"}}
			msg := "test message"

			mockLogger.EXPECT().With(gomock.Any()).DoAndReturn(func(v ...any) logger.Logger {
				expected := []any{"keys", keyvals}
				assert.Equal(t, expected, v)
				return withLogger
			})

			switch tc.levelMethod {
			case "Debug":
				withLogger.EXPECT().Debug(msg).Times(1)
			case "Info":
				withLogger.EXPECT().Info(msg).Times(1)
			case "Warn":
				withLogger.EXPECT().Warn(msg).Times(1)
			case "Error":
				withLogger.EXPECT().Error(msg).Times(1)
			}

			tc.logFunc(mockLogger, msg, keyvals...)
		})
	}
}
