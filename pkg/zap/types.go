package zap

import (
	"go.uber.org/zap"
)

type (
	Field          = zap.Field
	SamplingConfig = zap.SamplingConfig
	ZapOption      = zap.Option
)

var (
	Any = zap.Any
)
