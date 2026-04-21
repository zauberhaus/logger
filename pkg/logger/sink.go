package logger

import (
	"io"

	"go.uber.org/zap/zapcore"
)

type Sink interface {
	io.WriteCloser
	zapcore.WriteSyncer
}
