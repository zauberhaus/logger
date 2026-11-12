// Copyright 2026 Zauberhaus
// Licensed to Zauberhaus under one or more agreements.
// Zauberhaus licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package logger

import (
	"io"

	"go.uber.org/zap/zapcore"
)

type Sink interface {
	io.WriteCloser
	zapcore.WriteSyncer
}
