//go:generate go run go.uber.org/mock/mockgen@latest -package mock --typed --write_package_comment=false -destination=../mock/temporal_logger_mock.go -mock_names=Logger=MockTemporalLogger go.temporal.io/sdk/log Logger
package temporal

import (
	"github.com/zauberhaus/logger/pkg/logger"
	"go.temporal.io/sdk/log"
)

type TemporalLogger struct {
	logger logger.Logger
}

func NewLogger(logger logger.Logger) log.Logger {
	return &TemporalLogger{
		logger: logger,
	}
}

// Debug implements log.Logger
func (t *TemporalLogger) Debug(msg string, keyvals ...any) {
	t.logger.With("keys", keyvals).Debug(msg)
}

// Error implements log.Logger
func (t *TemporalLogger) Error(msg string, keyvals ...any) {
	t.logger.With("keys", keyvals).Error(msg)
}

// Info implements log.Logger
func (t *TemporalLogger) Info(msg string, keyvals ...any) {
	t.logger.With("keys", keyvals).Info(msg)
}

// Warn implements log.Logger
func (t *TemporalLogger) Warn(msg string, keyvals ...any) {
	t.logger.With("keys", keyvals).Warn(msg)
}
