package filtered

import (
	"github.com/zauberhaus/logger/pkg/logger"
	"github.com/zauberhaus/logger/pkg/zap"
	"go.uber.org/zap/zapcore"
)

type FilteredLogger struct {
	logger logger.Logger
	filter *Filter
}

func NewLogger(filter *Filter, options ...zap.Option) logger.Logger {
	if filter.Enabled() {
		checks := WithChecks(func(ent zapcore.Entry, ce *zapcore.CheckedEntry) bool {
			if !filter.Passed(ent.Message) {
				return false
			}

			return true
		})

		options = append(options, zap.WithZapOptions(checks))
	}

	return &FilteredLogger{
		logger: zap.NewLogger(options...),
		filter: filter,
	}
}

func (f *FilteredLogger) Debug(args ...any) {
	f.logger.Debug(args...)
}

func (f *FilteredLogger) Debugf(template string, args ...any) {
	f.logger.Debugf(template, args...)
}

func (f *FilteredLogger) EnableDebug() {
	f.logger.EnableDebug()
}

func (f *FilteredLogger) Error(args ...any) {
	f.logger.Error(args...)
}

func (f *FilteredLogger) Errorf(template string, args ...any) {
	f.logger.Errorf(template, args...)
}

func (f *FilteredLogger) Fatal(args ...any) {
	f.logger.Fatal(args...)
}

func (f *FilteredLogger) Fatalf(template string, args ...any) {
	f.logger.Fatalf(template, args...)
}

func (f *FilteredLogger) IsDebugEnabled() bool {
	return f.logger.IsDebugEnabled()
}

func (f *FilteredLogger) HasLevel(level logger.Level) bool {
	return f.logger.HasLevel(level)
}

func (f *FilteredLogger) Info(args ...any) {
	f.logger.Info(args...)
}

func (f *FilteredLogger) Infof(template string, args ...any) {
	f.logger.Infof(template, args...)
}

func (f *FilteredLogger) Level() logger.Level {
	return f.logger.Level()
}

func (f *FilteredLogger) Panic(args ...any) {
	f.logger.Panic(args...)
}

func (f *FilteredLogger) Panicf(template string, args ...any) {
	f.logger.Panicf(template, args...)
}

func (f *FilteredLogger) SetLevel(level logger.Level) {
	f.logger.SetLevel(level)
}

func (f *FilteredLogger) AddSkip(steps int) logger.Logger {
	return f.logger.AddSkip(steps)
}

func (f *FilteredLogger) Sync() error {
	return f.logger.Sync()
}

func (f *FilteredLogger) Warn(args ...any) {
	f.logger.Warn(args...)
}

func (f *FilteredLogger) Warnf(template string, args ...any) {
	f.logger.Warnf(template, args...)
}

func (f *FilteredLogger) With(args ...any) logger.Logger {
	return f.logger.With(args...)
}
