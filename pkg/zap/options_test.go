package zap

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zauberhaus/logger/pkg/logger"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestOptions(t *testing.T) {
	t.Run("Skip", func(t *testing.T) {
		opts := &ZapOptions{}
		Skip(5).Set(opts)
		assert.Equal(t, 5, opts.skip)
	})

	t.Run("WithoutCaller", func(t *testing.T) {
		opts := &ZapOptions{}
		WithoutCaller().Set(opts)
		assert.Equal(t, -1, opts.skip)
	})

	t.Run("WithLevel", func(t *testing.T) {
		opts := &ZapOptions{}
		WithLevel(logger.DebugLevel).Set(opts)
		assert.Equal(t, logger.DebugLevel, opts.level)
	})

	t.Run("WithZapOptions", func(t *testing.T) {
		opts := &ZapOptions{}
		zapOpt := zap.Hooks(func(e zapcore.Entry) error { return nil })
		WithZapOptions(zapOpt).Set(opts)
		assert.Len(t, opts.options, 1)
	})

	t.Run("WithSink", func(t *testing.T) {
		opts := &ZapOptions{}
		var buf bytes.Buffer
		WithSink(&buf).Set(opts)
		assert.Len(t, opts.sinks, 1)
		assert.Equal(t, &buf, opts.sinks[0])
	})

	t.Run("WithSinks", func(t *testing.T) {
		opts := &ZapOptions{}
		var buf1, buf2 bytes.Buffer
		WithSinks(&buf1, &buf2).Set(opts)
		assert.Len(t, opts.sinks, 2)
		assert.Equal(t, &buf1, opts.sinks[0])
		assert.Equal(t, &buf2, opts.sinks[1])
	})

	t.Run("WithWriteSyncer", func(t *testing.T) {
		opts := &ZapOptions{}
		ws := zapcore.AddSync(io.Discard)
		WithWriteSyncer(ws).Set(opts)
		assert.Len(t, opts.synker, 1)
		assert.Equal(t, ws, opts.synker[0])
	})

	t.Run("WithErrorSinks", func(t *testing.T) {
		opts := &ZapOptions{}
		var buf bytes.Buffer
		WithErrorSinks(&buf).Set(opts)
		assert.Len(t, opts.errorSinks, 1)
		assert.Equal(t, &buf, opts.errorSinks[0])
	})

	t.Run("WithStdOut", func(t *testing.T) {
		opts := &ZapOptions{}
		WithStdOut().Set(opts)
		assert.Len(t, opts.sinks, 1)
		assert.Equal(t, os.Stdout, opts.sinks[0])
	})

	t.Run("WithStdErr", func(t *testing.T) {
		opts := &ZapOptions{}
		WithStdErr().Set(opts)
		assert.Len(t, opts.errorSinks, 1)
		assert.Equal(t, os.Stdout, opts.errorSinks[0])
	})

	t.Run("WithOutput", func(t *testing.T) {
		opts := &ZapOptions{}
		WithOutput(JSONOutput).Set(opts)
		assert.Equal(t, JSONOutput, opts.output)
	})

	t.Run("WithFields", func(t *testing.T) {
		opts := &ZapOptions{}
		field := Any("key", "value")
		WithFields(field).Set(opts)
		assert.Len(t, opts.fields, 1)
		assert.Equal(t, field, opts.fields[0])
	})

	t.Run("WithField", func(t *testing.T) {
		opts := &ZapOptions{}
		WithField("key", "value").Set(opts)
		assert.Len(t, opts.fields, 1)
		expectedField := Any("key", "value")
		assert.Equal(t, expectedField, opts.fields[0])
	})

	t.Run("WithGenericOption", func(t *testing.T) {
		opts := &ZapOptions{}
		WithGenericOption("key", "value").Set(opts)
		assert.Equal(t, "value", opts.generic["key"])
	})

	t.Run("GetGenericOption", func(t *testing.T) {
		t.Run("found", func(t *testing.T) {
			val := GetGenericOption[string]("key", WithGenericOption("key", "value"))
			assert.Equal(t, "value", val)
		})

		t.Run("not found", func(t *testing.T) {
			val := GetGenericOption[string]("key")
			assert.Empty(t, val)
		})
	})

	t.Run("WithSampling", func(t *testing.T) {
		opts := &ZapOptions{}
		sampling := &SamplingConfig{}
		WithSampling(sampling).Set(opts)
		assert.Equal(t, sampling, opts.sampling)
	})

	t.Run("optionFunc", func(t *testing.T) {
		opts := &ZapOptions{}
		f := OptionFunc(func(o *ZapOptions) {
			o.skip = 10
		})
		f.Set(opts)
		assert.Equal(t, 10, opts.skip)
	})
}
