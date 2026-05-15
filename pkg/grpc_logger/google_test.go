// cspell:ignore Warningf Warningln Errorln Infoln
package grpc_logger_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zauberhaus/logger/pkg/grpc_logger"
	"github.com/zauberhaus/logger/pkg/logger"
	"github.com/zauberhaus/logger/pkg/memory"
	"github.com/zauberhaus/logger/pkg/mock"
	"github.com/zauberhaus/logger/pkg/zap"
	gomock "go.uber.org/mock/gomock"
	"google.golang.org/grpc/grpclog"
)

func TestNewGrpcLogger(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	l := grpc_logger.NewLogger(mock.NewMockLogger(ctrl))
	assert.NotNil(t, l)
}

func TestGrpcLogger_V(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLogger := mock.NewMockLogger(ctrl)

	mockLogger.EXPECT().HasLevel(logger.InfoLevel).Return(true).Times(1)
	mockLogger.EXPECT().IsDebugEnabled().Return(false).Times(1)

	l := grpc_logger.NewLogger(mockLogger)
	assert.True(t, l.V(0))
	assert.False(t, l.V(1))
}

func TestGrpcLogger_Levels(t *testing.T) {
	type logCall struct {
		name    string
		call    func(l grpclog.LoggerV2)
		setup   func(mock *mock.MockLogger)
		wantMsg string
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cases := []struct {
		name  string
		call  func(l grpclog.LoggerV2)
		setup func(m *mock.MockLogger)
	}{
		{
			name:  "Info",
			call:  func(l grpclog.LoggerV2) { l.Info("hello") },
			setup: func(m *mock.MockLogger) { m.EXPECT().Info("hello") },
		},
		{
			name:  "Infoln",
			call:  func(l grpclog.LoggerV2) { l.Infoln("hello") },
			setup: func(m *mock.MockLogger) { m.EXPECT().Info("hello") },
		},
		{
			name:  "Infof",
			call:  func(l grpclog.LoggerV2) { l.Infof("hello %s", "world") },
			setup: func(m *mock.MockLogger) { m.EXPECT().Infof("hello %s", "world") },
		},
		{
			name:  "Warning",
			call:  func(l grpclog.LoggerV2) { l.Warning("oops") },
			setup: func(m *mock.MockLogger) { m.EXPECT().Warn("oops") },
		},
		{
			name:  "Warningln",
			call:  func(l grpclog.LoggerV2) { l.Warningln("oops") },
			setup: func(m *mock.MockLogger) { m.EXPECT().Warn("oops") },
		},
		{
			name:  "Warningf",
			call:  func(l grpclog.LoggerV2) { l.Warningf("oops %d", 42) },
			setup: func(m *mock.MockLogger) { m.EXPECT().Warnf("oops %d", 42) },
		},
		{
			name:  "Error",
			call:  func(l grpclog.LoggerV2) { l.Error("bad") },
			setup: func(m *mock.MockLogger) { m.EXPECT().Error("bad") },
		},
		{
			name:  "Errorln",
			call:  func(l grpclog.LoggerV2) { l.Errorln("bad") },
			setup: func(m *mock.MockLogger) { m.EXPECT().Error("bad") },
		},
		{
			name:  "Errorf",
			call:  func(l grpclog.LoggerV2) { l.Errorf("bad %v", "thing") },
			setup: func(m *mock.MockLogger) { m.EXPECT().Errorf("bad %v", "thing") },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			m := mock.NewMockLogger(ctrl)
			tc.setup(m)
			tc.call(grpc_logger.NewLogger(m))
		})
	}

	_ = ctrl
}

func TestGrpcLogger_Memory(t *testing.T) {
	type message struct {
		Level   logger.Level `json:"level"`
		Message string       `json:"msg"`
		Time    time.Time    `json:"ts"`
		Caller  string       `json:"caller"`
	}

	m := memory.NewLogger(memory.WithBlocking(true), zap.WithOutput(zap.JSONOutput), zap.Skip(2))
	l := grpc_logger.NewLogger(m)
	assert.NotNil(t, l)

	l.Info("info message")

	txt := m.NextLine()
	var msg message
	err := json.NewDecoder(bytes.NewBufferString(txt)).Decode(&msg)
	if assert.NoError(t, err) {
		assert.Equal(t, logger.InfoLevel, msg.Level)
		assert.Equal(t, "info message", msg.Message)
		assert.NotEmpty(t, msg.Time)
		assert.Contains(t, msg.Caller, "grpc_logger/google_test.go:")
	}

	l.Warning("warn message")

	txt = m.NextLine()
	err = json.NewDecoder(bytes.NewBufferString(txt)).Decode(&msg)
	if assert.NoError(t, err) {
		assert.Equal(t, logger.WarnLevel, msg.Level)
		assert.Equal(t, "warn message", msg.Message)
	}

	l.Infof("hello %s", "world")

	txt = m.NextLine()
	err = json.NewDecoder(bytes.NewBufferString(txt)).Decode(&msg)
	if assert.NoError(t, err) {
		assert.Equal(t, logger.InfoLevel, msg.Level)
		assert.Equal(t, "hello world", msg.Message)
	}

	assert.Equal(t, 0, m.Len())
}
