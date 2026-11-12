// Copyright 2026 Zauberhaus
// Licensed to Zauberhaus under one or more agreements.
// Zauberhaus licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

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
