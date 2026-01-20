package filtered

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func WithChecks(checks ...checkFunc) zap.Option {
	return zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return &checked{
			core:  core,
			funcs: checks,
		}
	})
}
