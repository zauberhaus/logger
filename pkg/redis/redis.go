//go:generate go run go.uber.org/mock/mockgen@latest --typed --write_package_comment=false -destination=../mock/redis_logger_mock.go -source=./redis.go -package=mock

package redis

import (
	"context"
	"fmt"
	"strings"

	"github.com/zauberhaus/logger/pkg/logger"
)

type RedisLogger interface {
	Printf(ctx context.Context, format string, v ...any)
}

type redisLogger struct {
	logger logger.Logger
}

func NewLogger(logger logger.Logger) RedisLogger {
	if logger == nil {
		return nil
	}

	return &redisLogger{
		logger: logger,
	}
}

func (r *redisLogger) Printf(ctx context.Context, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	switch {
	case msg == "ERR max number of clients reached":
		r.logger.Error(msg)
	case strings.HasPrefix(msg, "LOADING "):
		r.logger.Warn(msg)
	case strings.HasPrefix(msg, "READONLY "):
		r.logger.Warn(msg)
	case strings.HasPrefix(msg, "CLUSTERDOWN "):
		r.logger.Warn(msg)
	case strings.HasPrefix(msg, "TRYAGAIN "):
		r.logger.Warn(msg)
	default:
		r.logger.Info(msg)
	}
}

type QuietRedisLogger struct {
	logger logger.Logger
}

func NewQuietLogger() RedisLogger {
	return &QuietRedisLogger{}
}

func (r *QuietRedisLogger) Printf(ctx context.Context, format string, args ...any) {
}
