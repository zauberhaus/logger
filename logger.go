package logger

import (
	"context"

	"github.com/zauberhaus/logger/pkg/zap"
)

var (
	logger = zap.NewLogger(zap.WithLevel(InfoLevel), zap.Skip(2))
)

func SetLogger(l Logger) {
	logger = l
}

func GetLogger(ctx context.Context) Logger {
	if ctx != nil {
		if val := ctx.Value(LoggerKey); val != nil {
			if l, ok := val.(Logger); ok {
				return l
			}
		}
	}

	return logger
}

func AddLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, LoggerKey, logger)
}
