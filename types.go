package logger

import log "github.com/zauberhaus/logger/pkg/logger"

type (
	ContextKey string

	Logger = log.Logger
	Level  = log.Level
)

const (
	LoggerKey = ContextKey("logger")

	DebugLevel = log.DebugLevel
	InfoLevel  = log.InfoLevel
	WarnLevel  = log.WarnLevel
	ErrorLevel = log.ErrorLevel
	PanicLevel = log.PanicLevel
	FatalLevel = log.FatalLevel
)
