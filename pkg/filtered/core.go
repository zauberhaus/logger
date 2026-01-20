package filtered

import (
	"go.uber.org/zap/zapcore"
)

type checkFunc func(ent zapcore.Entry, ce *zapcore.CheckedEntry) bool

type checked struct {
	core  zapcore.Core
	funcs []checkFunc
}

var _ zapcore.Core = (zapcore.Core)(nil)

func (h *checked) Level() zapcore.Level {
	return zapcore.LevelOf(h.core)
}

func (h *checked) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	for _, c := range h.funcs {
		if !c(ent, ce) {
			return nil
		}
	}

	return h.core.Check(ent, ce)
}

func (h *checked) With(fields []zapcore.Field) zapcore.Core {
	return &checked{
		core:  h.core.With(fields),
		funcs: h.funcs,
	}
}

func (h *checked) Write(ent zapcore.Entry, f []zapcore.Field) error {
	return h.core.Write(ent, f)
}

func (h *checked) Enabled(level zapcore.Level) bool {
	return h.core.Enabled(level)
}

func (h *checked) Sync() error {
	return h.core.Sync()
}
