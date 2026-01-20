package filtered_test

import (
	"bytes"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	"github.com/zauberhaus/logger/pkg/filtered"
	"github.com/zauberhaus/logger/pkg/logger"
	zapLogger "github.com/zauberhaus/logger/pkg/zap"
	"go.uber.org/zap/zapcore"
)

func TestFilteredLogger_Levels(t *testing.T) {
	include := &filtered.Filter{
		Include: []*regexp.Regexp{regexp.MustCompile("allow")},
	}

	exclude := &filtered.Filter{
		Exclude: []*regexp.Regexp{regexp.MustCompile("block")},
	}

	testCases := []struct {
		name      string
		logFunc   func(l logger.Logger, msg string)
		logfFunc  func(l logger.Logger, format string, args ...any)
		level     logger.Level
		levelText string
	}{
		{"Debug", func(l logger.Logger, msg string) { l.Debug(msg) }, func(l logger.Logger, format string, args ...any) { l.Debugf(format, args...) }, logger.DebugLevel, "debug"},
		{"Info", func(l logger.Logger, msg string) { l.Info(msg) }, func(l logger.Logger, format string, args ...any) { l.Infof(format, args...) }, logger.InfoLevel, "info"},
		{"Warn", func(l logger.Logger, msg string) { l.Warn(msg) }, func(l logger.Logger, format string, args ...any) { l.Warnf(format, args...) }, logger.WarnLevel, "warn"},
		{"Error", func(l logger.Logger, msg string) { l.Error(msg) }, func(l logger.Logger, format string, args ...any) { l.Errorf(format, args...) }, logger.ErrorLevel, "error"},
		{"Panic", func(l logger.Logger, msg string) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("The code did not panic")
				}
			}()
			l.Panic(msg)
		}, func(l logger.Logger, format string, args ...any) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("The code did not panic")
				}
			}()
			l.Panicf(format, args...)
		}, logger.PanicLevel, "panic"},
		{"Fatal", func(l logger.Logger, msg string) { l.Fatal(msg) }, func(l logger.Logger, format string, args ...any) { l.Fatalf(format, args...) }, logger.FatalLevel, "fatal"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for i, filter := range []*filtered.Filter{include, exclude} {
				name := "include"
				if i > 0 {
					name = "exclude"
				}

				t.Run(name, func(t *testing.T) {

					exit := false

					var buf bytes.Buffer

					l := filtered.NewLogger(filter, zapLogger.WithSinks(&buf), zapLogger.WithLevel(tc.level))

					switch tc.level {
					case logger.DebugLevel:
						assert.True(t, l.IsDebugEnabled())
					case logger.FatalLevel:
						fakeExit := func(code int) {
							exit = true
						}
						gomonkey.ApplyFunc(os.Exit, fakeExit)
					default:
						assert.False(t, l.IsDebugEnabled())
					}

					tc.logFunc(l, "allow this")
					tc.logFunc(l, "block this")

					tc.logfFunc(l, "allow %s", "that")
					tc.logfFunc(l, "block %s", "that")

					output := buf.String()
					assert.Contains(t, output, "allow this")
					assert.NotContains(t, output, "block this")
					assert.Contains(t, output, "allow that")
					assert.NotContains(t, output, "block that")

					if tc.level == logger.FatalLevel {
						assert.True(t, exit)
					} else {
						assert.False(t, exit)
					}
				})
			}
		})
	}
}

func TestFilteredLogger_With(t *testing.T) {
	var buf bytes.Buffer
	filter := &filtered.Filter{
		Include: []*regexp.Regexp{regexp.MustCompile("allow")},
	}

	l := filtered.NewLogger(filter, zapLogger.WithSinks(&buf), zapLogger.WithOutput(zapLogger.JSONOutput))
	l = l.With("key", "value")

	l.Info("allow this")

	output := buf.String()
	assert.Contains(t, output, `"key":"value"`)
	assert.Contains(t, output, "allow this")
}

func TestFilteredLogger_Skip(t *testing.T) {
	var buf bytes.Buffer
	filter := &filtered.Filter{
		Include: []*regexp.Regexp{regexp.MustCompile("allow")},
	}

	l := filtered.NewLogger(filter, zapLogger.WithSinks(&buf))
	l = l.AddSkip(1)

	l.Info("allow this")

	output := buf.String()
	// The caller information is hard to assert precisely, but we can check that the message is logged.
	assert.Contains(t, output, "allow this")
}

func TestFilteredLogger_NoFilter(t *testing.T) {
	var buf bytes.Buffer
	filter := &filtered.Filter{}

	l := filtered.NewLogger(filter, zapLogger.WithSinks(&buf))

	l.Info("message 1")
	l.Info("message 2")

	output := buf.String()
	assert.Contains(t, output, "message 1")
	assert.Contains(t, output, "message 2")
}

func TestFilteredLogger_EmptyFilter(t *testing.T) {
	var buf bytes.Buffer
	filter := &filtered.Filter{
		Include: []*regexp.Regexp{},
		Exclude: []*regexp.Regexp{},
	}

	l := filtered.NewLogger(filter, zapLogger.WithSinks(&buf))

	l.Info("message 1")

	output := buf.String()
	assert.Contains(t, output, "message 1")
}

func TestFilteredLogger_EnableDebug(t *testing.T) {
	var buf bytes.Buffer
	filter := &filtered.Filter{
		Include: []*regexp.Regexp{},
		Exclude: []*regexp.Regexp{},
	}

	l := filtered.NewLogger(filter, zapLogger.WithSinks(&buf))

	lvl := l.Level()
	assert.Equal(t, logger.InfoLevel, lvl)

	assert.True(t, l.HasLevel(logger.WarnLevel))
	assert.True(t, l.HasLevel(logger.InfoLevel))
	assert.False(t, l.HasLevel(logger.DebugLevel))

	l.SetLevel(logger.WarnLevel)

	assert.True(t, l.HasLevel(logger.WarnLevel))
	assert.False(t, l.HasLevel(logger.InfoLevel))
	assert.False(t, l.HasLevel(logger.DebugLevel))

	l.EnableDebug()

	l.Debug("message 1")

	output := buf.String()
	assert.Contains(t, output, "message 1")
}

func TestFilteredLogger_IncludeAndExclude(t *testing.T) {
	var buf bytes.Buffer
	filter := &filtered.Filter{
		Include: []*regexp.Regexp{regexp.MustCompile("allow")},
		Exclude: []*regexp.Regexp{regexp.MustCompile("block")},
	}

	l := filtered.NewLogger(filter, zapLogger.WithSinks(&buf))

	l.Info("allow this")
	l.Info("block this")

	output := buf.String()
	assert.Contains(t, output, "allow this")
	assert.NotContains(t, output, "block this")
}

func TestFilteredLogger_Sync(t *testing.T) {

	var buf bytes.Buffer
	ws := zapcore.AddSync(&buf)

	// Configure the buffered writer syncer
	bws := &zapcore.BufferedWriteSyncer{
		WS:            ws,
		Size:          512 * 1024,       // Custom buffer size (512 kB)
		FlushInterval: 60 * time.Minute, // Custom flush interval (1 minute)
	}

	l := filtered.NewLogger(nil, zapLogger.WithWriteSyncer(bws))
	l.Info("test message")

	assert.Equal(t, 0, buf.Len())

	l.Sync()

	assert.NotEqual(t, 0, buf.Len())

}
