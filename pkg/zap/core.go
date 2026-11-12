// Copyright 2026 Zauberhaus
// Licensed to Zauberhaus under one or more agreements.
// Zauberhaus licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package zap

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type levelCore struct {
	zapcore.Core
	level zap.AtomicLevel
}

func newLevelCore(core zapcore.Core, level zap.AtomicLevel) *levelCore {
	return &levelCore{
		Core:  core,
		level: level,
	}
}

func (c *levelCore) Enabled(lvl zapcore.Level) bool {
	return c.level.Enabled(lvl)
}

func (c *levelCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}

	return ce
}

func (c *levelCore) With(fields []zapcore.Field) zapcore.Core {
	return &levelCore{
		Core:  c.Core.With(fields),
		level: c.level,
	}
}
