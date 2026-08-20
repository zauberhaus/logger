package zap

import (
	"go.uber.org/zap/zapcore"
)

// splitCore fans entries out to one of two underlying cores based on
// level, dispatching in Write rather than relying on zapcore.NewTee's
// per-child Check: this package's levelCore.Check adds itself (not its
// wrapped Core) to the zapcore.CheckedEntry, so a Tee nested underneath it
// would never get its own Check called, and its Write unconditionally
// writes to every child regardless of level. Doing the level test directly
// in Write sidesteps that.
type splitCore struct {
	low, high  zapcore.Core
	splitLevel zapcore.Level
}

func newSplitCore(encoder zapcore.Encoder, low, high zapcore.WriteSyncer, splitLevel zapcore.Level) *splitCore {
	return &splitCore{
		low:        zapcore.NewCore(encoder, low, zapcore.DebugLevel),
		high:       zapcore.NewCore(encoder, high, zapcore.DebugLevel),
		splitLevel: splitLevel,
	}
}

func (c *splitCore) Enabled(zapcore.Level) bool {
	return true
}

func (c *splitCore) With(fields []zapcore.Field) zapcore.Core {
	return &splitCore{
		low:        c.low.With(fields),
		high:       c.high.With(fields),
		splitLevel: c.splitLevel,
	}
}

func (c *splitCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return ce.AddCore(ent, c)
}

func (c *splitCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	if ent.Level >= c.splitLevel {
		return c.high.Write(ent, fields)
	}
	return c.low.Write(ent, fields)
}

func (c *splitCore) Sync() error {
	lowErr := c.low.Sync()
	highErr := c.high.Sync()
	if lowErr != nil {
		return lowErr
	}
	return highErr
}
