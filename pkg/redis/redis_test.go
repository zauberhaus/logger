package redis_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zauberhaus/logger/pkg/logger"
	"github.com/zauberhaus/logger/pkg/memory"
	"github.com/zauberhaus/logger/pkg/mock"
	"github.com/zauberhaus/logger/pkg/redis"
	"github.com/zauberhaus/logger/pkg/zap"
	"go.uber.org/mock/gomock"

	rc "github.com/redis/go-redis/v9"
)

func TestNewRedisLogger(t *testing.T) {
	t.Run("nil logger", func(t *testing.T) {
		l := redis.NewLogger(nil)
		if l != nil {
			t.Errorf("Expected nil, got %v", l)
		}
	})

	t.Run("non-nil logger", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockLogger := mock.NewMockLogger(ctrl)
		l := redis.NewLogger(mockLogger)
		if l == nil {
			t.Errorf("Expected non-nil, got nil")
		}
	})
}

func TestRedisMemoryLogger(t *testing.T) {
	type message struct {
		Level      logger.Level `json:"level"`
		Message    string       `json:"msg"`
		Time       time.Time    `json:"ts"`
		Caller     string       `json:"caller"`
		Stacktrace string       `json:"stacktrace"`
	}

	testCases := []struct {
		name        string
		message     string
		expectLevel logger.Level
	}{
		{"clients reached", "ERR max number of clients reached", logger.ErrorLevel},
		{"loading", "LOADING something", logger.WarnLevel},
		{"readonly", "READONLY something", logger.WarnLevel},
		{"clusterdown", "CLUSTERDOWN something", logger.WarnLevel},
		{"tryagain", "TRYAGAIN something", logger.WarnLevel},
		{"default", "some other message", logger.InfoLevel},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := memory.NewLogger(memory.WithBlocking(true), zap.WithOutput(zap.JSONOutput), zap.Skip(2))
			l := redis.NewLogger(m)
			assert.NotNil(t, l)

			l.Printf(context.Background(), tc.message)

			txt := m.NextLine()
			var msg message
			decoder := json.NewDecoder(bytes.NewBuffer([]byte(txt)))
			decoder.DisallowUnknownFields()

			err := decoder.Decode(&msg)
			if assert.NoError(t, err) {
				assert.NotEmpty(t, msg.Time)
				assert.Equal(t, tc.message, msg.Message)
				assert.Contains(t, msg.Caller, "redis/redis_test.go:")
				assert.Equal(t, tc.expectLevel, msg.Level)

				if tc.expectLevel == logger.ErrorLevel {
					assert.NotEmpty(t, msg.Stacktrace)
				} else {
					assert.Empty(t, msg.Stacktrace)
				}
			}
		})
	}
}

func TestRedisLogger_Printf(t *testing.T) {
	testCases := []struct {
		name        string
		message     string
		expectLevel string
	}{
		{"clients reached", "ERR max number of clients reached", "Error"},
		{"loading", "LOADING something", "Warn"},
		{"readonly", "READONLY something", "Warn"},
		{"clusterdown", "CLUSTERDOWN something", "Warn"},
		{"tryagain", "TRYAGAIN something", "Warn"},
		{"default", "some other message", "Info"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockLogger := mock.NewMockLogger(ctrl)

			switch tc.expectLevel {
			case "Error":
				mockLogger.EXPECT().Error(tc.message).Times(1)
			case "Warn":
				mockLogger.EXPECT().Warn(tc.message).Times(1)
			case "Info":
				mockLogger.EXPECT().Info(tc.message).Times(1)
			}

			l := redis.NewLogger(mockLogger)
			l.Printf(context.Background(), tc.message)
		})
	}
}

func TestQuietRedisLogger_Printf(t *testing.T) {
	// No mocks needed as it should do nothing.
	l := redis.NewQuietLogger()
	l.Printf(context.Background(), "test message")
	// No assertions needed, we just need to ensure it doesn't panic.
}

func TestCheckRedis(t *testing.T) {
	l := redis.NewLogger(nil)
	rc.SetLogger(l)

	l = redis.NewQuietLogger()
	rc.SetLogger(l)
}
