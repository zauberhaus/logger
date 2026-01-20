package sentry

import (
	"github.com/getsentry/sentry-go"
	"github.com/zauberhaus/logger/pkg/logger"
)

func Level(level logger.Level) sentry.Level {
	switch level {
	case logger.PanicLevel, logger.FatalLevel:
		return sentry.LevelFatal
	case logger.ErrorLevel:
		return sentry.LevelError
	case logger.WarnLevel:
		return sentry.LevelWarning
	case logger.InfoLevel:
		return sentry.LevelInfo
	case logger.DebugLevel:
		return sentry.LevelDebug
	default:
		return sentry.LevelInfo
	}
}

func LogLevel(level sentry.Level) logger.Level {
	switch level {
	case sentry.LevelFatal:
		return logger.FatalLevel
	case sentry.LevelError:
		return logger.ErrorLevel
	case sentry.LevelWarning:
		return logger.WarnLevel
	case sentry.LevelInfo:
		return logger.InfoLevel
	case sentry.LevelDebug:
		return logger.DebugLevel
	default:
		return logger.InfoLevel
	}
}
