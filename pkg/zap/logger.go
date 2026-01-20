package zap

import (
	"fmt"
	"os"
	"time"

	"github.com/zauberhaus/logger/pkg/logger"
	"github.com/zauberhaus/logger/pkg/sentry"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ZapLogger struct {
	logger *zap.SugaredLogger
	level  zap.AtomicLevel
	hub    *sentry.Hub
}

func NewLogger(options ...Option) logger.Logger {
	o := &ZapOptions{
		level: logger.InfoLevel,
		skip:  1,
	}

	for _, opt := range options {
		if opt != nil {
			opt.Set(o)
		}
	}

	if o.skip == -1 {
		o.options = append(o.options, zap.WithCaller(false))
	} else {
		o.options = append(o.options, zap.AddCaller(), zap.AddCallerSkip(o.skip))
	}

	l := Level(o.level)
	enabler := zap.NewAtomicLevelAt(l)

	encoderCfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,
		EncodeTime:     zapcore.RFC3339TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	var logger *zap.Logger
	var ws zapcore.WriteSyncer

	for _, sink := range o.sinks {
		o.synker = append(o.synker, zapcore.AddSync(sink))
	}

	switch len(o.synker) {
	case 0:
		ws = zapcore.AddSync(os.Stdout)
	case 1:
		ws = o.synker[0]
	default:
		var s []zapcore.WriteSyncer
		for _, sync := range o.synker {
			s = append(s, sync)
		}

		ws = zapcore.NewMultiWriteSyncer(s...)
	}

	var eo zapcore.WriteSyncer
	switch len(o.errorSinks) {
	case 0:
	case 1:
		eo = zapcore.AddSync(o.errorSinks[0])
	default:
		var s []zapcore.WriteSyncer
		for _, sink := range o.errorSinks {
			s = append(s, zapcore.AddSync(sink))
		}

		eo = zapcore.NewMultiWriteSyncer(s...)
	}

	if eo != nil {
		o.options = append(o.options, zap.ErrorOutput(eo))
	}

	o.options = append(o.options, zap.AddStacktrace(zap.ErrorLevel))

	if o.sampling != nil {
		o.options = append(o.options, zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			var samplerOpts []zapcore.SamplerOption
			if o.sampling.Hook != nil {
				samplerOpts = append(samplerOpts, zapcore.SamplerHook(o.sampling.Hook))
			}
			return zapcore.NewSamplerWithOptions(
				core,
				time.Second,
				o.sampling.Initial,
				o.sampling.Thereafter,
				samplerOpts...,
			)
		}))

	}

	var encoder zapcore.Encoder

	switch o.output {
	case JSONOutput:
		encoderCfg.EncodeLevel = zapcore.LowercaseLevelEncoder
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	default:
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	}

	core := zapcore.NewCore(encoder, ws, enabler)
	logger = zap.New(core, o.options...)

	if len(o.fields) > 0 {
		logger = logger.With(o.fields...)
	}

	return &ZapLogger{
		logger.Sugar(),
		enabler,
		o.hub,
	}
}

func (z *ZapLogger) With(args ...any) logger.Logger {
	return &ZapLogger{
		z.logger.With(args...),
		z.level,
		z.hub,
	}
}

func (z *ZapLogger) AddSkip(skip int) logger.Logger {

	caller := zap.AddCallerSkip(skip)

	return &ZapLogger{
		logger: z.logger.WithOptions(caller),
		level:  z.level,
		hub:    z.hub,
	}
}

func (z *ZapLogger) SetLevel(level logger.Level) {
	l := Level(level)
	z.level.SetLevel(l)
}

func (z *ZapLogger) Level() logger.Level {
	lvl := LogLevel(z.level.Level())
	return lvl
}

func (z *ZapLogger) HasLevel(level logger.Level) bool {
	return z.Level() <= level
}

func (z *ZapLogger) EnableDebug() {
	z.SetLevel(logger.DebugLevel)
}

func (z *ZapLogger) IsDebugEnabled() bool {
	return z.HasLevel(logger.DebugLevel)
}

func (z *ZapLogger) Debug(args ...any) {
	z.logger.Debug(args...)
}

func (z *ZapLogger) Info(args ...any) {
	z.logger.Info(args...)
}

func (z *ZapLogger) Warn(args ...any) {
	z.hub.Message(logger.WarnLevel, args...)
	z.logger.Warn(args...)
}

func (z *ZapLogger) Error(args ...any) {
	z.hub.Capture(args...)
	z.logger.Error(args...)
}

func (z *ZapLogger) Panic(args ...any) {
	z.hub.Capture(args...)
	z.logger.Panic(args...)
}

func (z *ZapLogger) Fatal(args ...any) {
	z.hub.Capture(args...)
	z.logger.Fatal(args...)
}

func (z *ZapLogger) Debugf(template string, args ...any) {
	z.logger.Debugf(template, args...)
}

func (z *ZapLogger) Infof(template string, args ...any) {
	z.logger.Infof(template, args...)
}

func (z *ZapLogger) Warnf(template string, args ...any) {
	msg := fmt.Sprintf(template, args...)
	z.hub.Message(logger.WarnLevel, msg)

	z.logger.Warnf(template, args...)
}

func (z *ZapLogger) Errorf(template string, args ...any) {
	z.hub.Capture(args...)
	z.logger.Errorf(template, args...)
}

func (z *ZapLogger) Panicf(template string, args ...any) {
	z.hub.Capture(args...)
	z.logger.Panicf(template, args...)
}

func (z *ZapLogger) Fatalf(template string, args ...any) {
	z.hub.Capture(args...)
	z.logger.Fatalf(template, args...)
}

func (z *ZapLogger) Sync() error {
	return z.logger.Sync()
}
