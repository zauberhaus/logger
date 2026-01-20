package zap

import (
	"io"
	"os"

	"github.com/zauberhaus/logger/pkg/logger"
	"github.com/zauberhaus/logger/pkg/sentry"

	"go.uber.org/zap/zapcore"
)

type Output int

const (
	ConsoleOutput Output = iota
	JSONOutput
)

type ZapOptions struct {
	level      logger.Level
	skip       int
	options    []ZapOption
	sinks      []io.Writer
	synker     []zapcore.WriteSyncer
	errorSinks []io.Writer
	output     Output
	hub        *sentry.Hub

	fields  []Field
	generic map[string]any

	sampling *SamplingConfig
}

type Option interface {
	Set(*ZapOptions)
}

type OptionFunc func(o *ZapOptions)

func (f OptionFunc) Set(o *ZapOptions) {
	f(o)
}

func Skip(val int) Option {
	return OptionFunc(func(o *ZapOptions) {
		o.skip = val
	})
}

func WithoutCaller() Option {
	return OptionFunc(func(o *ZapOptions) {
		o.skip = -1
	})
}

func WithLevel(val logger.Level) Option {
	return OptionFunc(func(o *ZapOptions) {
		o.level = val
	})
}

func WithZapOptions(vals ...ZapOption) Option {
	return OptionFunc(func(o *ZapOptions) {
		o.options = append(o.options, vals...)
	})
}

func WithSink(val io.Writer) Option {
	return OptionFunc(func(o *ZapOptions) {
		o.sinks = []io.Writer{val}
	})
}

func WithSinks(vals ...io.Writer) Option {
	return OptionFunc(func(o *ZapOptions) {
		o.sinks = append(o.sinks, vals...)
	})
}

func WithWriteSyncer(vals ...zapcore.WriteSyncer) Option {
	return OptionFunc(func(o *ZapOptions) {
		o.synker = append(o.synker, vals...)
	})
}

func WithErrorSinks(vals ...io.Writer) Option {
	return OptionFunc(func(o *ZapOptions) {
		o.errorSinks = append(o.errorSinks, vals...)
	})
}

func WithStdOut() Option {
	return OptionFunc(func(o *ZapOptions) {
		o.sinks = append(o.sinks, os.Stdout)
	})
}

func WithStdErr() Option {
	return OptionFunc(func(o *ZapOptions) {
		o.errorSinks = append(o.errorSinks, os.Stdout)
	})
}

func WithOutput(val Output) Option {
	return OptionFunc(func(o *ZapOptions) {
		o.output = val
	})
}

func WithFields(vals ...Field) Option {
	return OptionFunc(func(o *ZapOptions) {
		o.fields = append(o.fields, vals...)
	})
}

func WithField(key string, val any) Option {
	return OptionFunc(func(o *ZapOptions) {
		field := Any(key, val)
		o.fields = append(o.fields, field)
	})
}

func WithGenericOption(key string, val any) Option {
	return OptionFunc(func(o *ZapOptions) {
		if o.generic == nil {
			o.generic = make(map[string]any)
		}

		o.generic[key] = val
	})
}

func GetGenericOption[T any](key string, options ...Option) T {
	o := &ZapOptions{
		skip: -1,
	}

	for _, opt := range options {
		if opt != nil {
			opt.Set(o)
		}
	}

	if v, ok := o.generic[key]; ok {
		if val, ok := v.(T); ok {
			return val
		}
	}

	return *new(T)
}

func WithSampling(val *SamplingConfig) Option {
	return OptionFunc(func(o *ZapOptions) {
		o.sampling = val
	})
}
