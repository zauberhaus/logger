package zap

import (
	"github.com/zauberhaus/logger/pkg/logger"
	"go.uber.org/zap/zapcore"
)

func Level(level logger.Level) zapcore.Level {
	switch level {
	case logger.FatalLevel:
		return zapcore.FatalLevel
	case logger.PanicLevel:
		return zapcore.PanicLevel
	case logger.ErrorLevel:
		return zapcore.ErrorLevel
	case logger.WarnLevel:
		return zapcore.WarnLevel
	case logger.InfoLevel:
		return zapcore.InfoLevel
	case logger.DebugLevel:
		return zapcore.DebugLevel
	default:
		return zapcore.InfoLevel
	}
}

func LogLevel(level zapcore.Level) logger.Level {
	switch level {
	case zapcore.FatalLevel:
		return logger.FatalLevel
	case zapcore.PanicLevel:
		return logger.PanicLevel
	case zapcore.ErrorLevel:
		return logger.ErrorLevel
	case zapcore.WarnLevel:
		return logger.WarnLevel
	case zapcore.InfoLevel:
		return logger.InfoLevel
	case zapcore.DebugLevel:
		return logger.DebugLevel
	default:
		return logger.InfoLevel
	}
}
