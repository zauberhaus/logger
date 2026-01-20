package sentry

import (
	"github.com/getsentry/sentry-go"
	"github.com/zauberhaus/logger/pkg/logger"
	"github.com/zauberhaus/logger/pkg/zap"
	"go.uber.org/zap/zapcore"
)

func WithSentry(s *sentry.Hub) zap.Option {
	hub := New(s)
	f := func(entry zapcore.Entry) error {
		switch entry.Level {
		case zapcore.WarnLevel:
			hub.Message(logger.WarnLevel, entry.Message)
		}

		return nil
	}

	return zap.OptionFunc(func(o *zap.ZapOptions) {
		o.hub = hub
		o.options = append(o.options, zap.Hooks(f))
	})
}
