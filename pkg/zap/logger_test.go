package zap_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zauberhaus/logger/pkg/logger"
	"github.com/zauberhaus/logger/pkg/zap"
	"go.uber.org/zap/zapcore"

	"github.com/agiledragon/gomonkey/v2"
)

type writer struct {
}

var _ io.Writer = &writer{}

func (w *writer) Write(p []byte) (n int, err error) {
	return 0, fmt.Errorf("write error")
}

func TestNewZapLogger(t *testing.T) {
	l := zap.NewLogger()
	assert.NotNil(t, l)
}

func TestZapLogger_Levels(t *testing.T) {

	format := "%s"

	testCases := []struct {
		name    string
		level   logger.Level
		logFunc func(l logger.Logger, msg string)
	}{
		{
			name:    "Debug",
			level:   logger.DebugLevel,
			logFunc: func(l logger.Logger, msg string) { l.Debug(msg) },
		},
		{
			name:    "Debugf",
			level:   logger.DebugLevel,
			logFunc: func(l logger.Logger, msg string) { l.Debugf(format, msg) },
		},
		{
			name:    "Info",
			level:   logger.InfoLevel,
			logFunc: func(l logger.Logger, msg string) { l.Info(msg) },
		},
		{
			name:    "Infof",
			level:   logger.InfoLevel,
			logFunc: func(l logger.Logger, msg string) { l.Infof(format, msg) },
		},
		{
			name:    "Warn",
			level:   logger.WarnLevel,
			logFunc: func(l logger.Logger, msg string) { l.Warn(msg) },
		},
		{
			name:    "Warnf",
			level:   logger.WarnLevel,
			logFunc: func(l logger.Logger, msg string) { l.Warnf(format, msg) },
		},
		{
			name:    "Error",
			level:   logger.ErrorLevel,
			logFunc: func(l logger.Logger, msg string) { l.Error(msg) },
		},
		{
			name:    "Errorf",
			level:   logger.ErrorLevel,
			logFunc: func(l logger.Logger, msg string) { l.Errorf(format, msg) },
		},
		{
			name:    "Panic",
			level:   logger.PanicLevel,
			logFunc: func(l logger.Logger, msg string) { l.Panic(msg) },
		},
		{
			name:    "Panicf",
			level:   logger.PanicLevel,
			logFunc: func(l logger.Logger, msg string) { l.Panicf(format, msg) },
		},
		{
			name:    "Fatal",
			level:   logger.FatalLevel,
			logFunc: func(l logger.Logger, msg string) { l.Fatal(msg) },
		},
		{
			name:    "Fatalf",
			level:   logger.FatalLevel,
			logFunc: func(l logger.Logger, msg string) { l.Fatalf(format, msg) },
		},
	}

	for _, tc := range testCases {
		var lock sync.Mutex

		t.Run(tc.name, func(t *testing.T) {
			lock.Lock()
			defer lock.Unlock()

			exit := false

			switch tc.level {
			case logger.PanicLevel:
				orig := tc.logFunc

				f := func(l logger.Logger, msg string) {
					defer func() {
						if r := recover(); r != nil {
							exit = true
						}
					}()
					orig(l, msg)
				}

				tc.logFunc = f
			case logger.FatalLevel:
				fakeExit := func(code int) {
					exit = true
				}
				gomonkey.ApplyFunc(os.Exit, fakeExit)
			default:
				exit = true
			}

			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			//old2 := os.Stderr
			//r2, w2, _ := os.Pipe()
			//os.Stderr = w2

			msg := "test message"

			l := zap.NewLogger(zap.WithLevel(tc.level))
			tc.logFunc(l, msg)

			w.Close()
			os.Stdout = old

			//w2.Close()
			//os.Stderr = old2

			var buf bytes.Buffer
			io.Copy(&buf, r)

			//var buf2 bytes.Buffer
			//io.Copy(&buf2, r2)

			stdout := buf.String()
			//stderr := buf2.String()

			//_ = stderr

			assert.Contains(t, stdout, msg)
			assert.Contains(t, stdout, strings.ToUpper(tc.level.String()))
			assert.True(t, exit)
		})
	}
}

func TestZapLogger_SetLevel(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	l := zap.NewLogger(zap.WithLevel(logger.InfoLevel))
	l.Debug("should not see this")

	l.SetLevel(logger.DebugLevel)
	l.Debug("should see this")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()
	assert.NotContains(t, output, "should not see this")
	assert.Contains(t, output, "should see this")
}

func TestZapLogger_With(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	l := zap.NewLogger(zap.WithLevel(logger.InfoLevel))
	l = l.With("key", "value")
	l.Info("test message")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()
	assert.Contains(t, output, "test message")
	assert.Contains(t, output, `"key": "value"`)
}

func TestZapLogger_Skip(t *testing.T) {
	var buf bytes.Buffer

	type message struct {
		Level      logger.Level
		Message    string    `json:"msg"`
		Time       time.Time `json:"ts"`
		Caller     string    `json:"caller"`
		Stacktrace string    `json:"stacktrace"`
	}

	l := zap.NewLogger(zap.WithSink(&buf), zap.WithOutput(zap.JSONOutput))
	l.Info("test message 1")

	l = l.AddSkip(-1)
	l.Info("test message 2")

	scanner := bufio.NewScanner(&buf)

	var msgs []*message

	for scanner.Scan() {
		output1 := scanner.Text()

		decoder := json.NewDecoder(bytes.NewBuffer([]byte(output1)))
		decoder.DisallowUnknownFields()

		var msg message
		err := decoder.Decode(&msg)
		if assert.NoError(t, err) {
			msgs = append(msgs, &msg)
		}
	}

	assert.Len(t, msgs, 2)
	assert.Contains(t, msgs[0].Caller, "zap/logger_test.go:")
	assert.Contains(t, msgs[1].Caller, "zap/logger.go:")
}

func TestZapLogger_MultiSync(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	var b1 bytes.Buffer
	var b2 bytes.Buffer

	l := zap.NewLogger(zap.WithLevel(logger.InfoLevel), zap.WithSinks(&b1, &b2), zap.WithStdOut(), zap.WithOutput(zap.JSONOutput))
	l = l.With("key", "value")
	l.Info("test message")
	l.Error("test error")

	w.Close()
	os.Stdout = old

	var b3 bytes.Buffer
	io.Copy(&b3, r)

	output1 := b1.String()
	output2 := b2.String()
	output3 := b3.String()

	assert.Equal(t, output1, output2)
	assert.Equal(t, output1, output3)

	type message struct {
		Key        string
		Level      logger.Level
		Message    string    `json:"msg"`
		Time       time.Time `json:"ts"`
		Caller     string    `json:"caller"`
		Stacktrace string    `json:"stacktrace"`
	}

	scanner := bufio.NewScanner(&b1)
	cnt := 0

	for scanner.Scan() {
		decoder := json.NewDecoder(bytes.NewBuffer(scanner.Bytes()))
		decoder.DisallowUnknownFields()

		var msg message
		err := decoder.Decode(&msg)
		assert.NoError(t, err)

		switch cnt {
		case 0:
			assert.Equal(t, msg.Key, "value")
			assert.Equal(t, msg.Level, logger.InfoLevel)
			assert.Equal(t, msg.Message, "test message")
			assert.Contains(t, msg.Caller, "zap/logger_test.go:")
			assert.Empty(t, msg.Stacktrace)
		case 1:
			assert.Equal(t, msg.Key, "value")
			assert.Equal(t, msg.Level, logger.ErrorLevel)
			assert.Equal(t, msg.Message, "test error")
			assert.Contains(t, msg.Caller, "zap/logger_test.go:")
			assert.NotEmpty(t, msg.Stacktrace)
		}

		cnt++
	}

	assert.Equal(t, 2, cnt)
}

func TestZapLogger_JsonOutput(t *testing.T) {
	var b1 bytes.Buffer

	type message struct {
		Key        string
		Level      logger.Level
		Message    string    `json:"msg"`
		Time       time.Time `json:"ts"`
		Caller     string    `json:"caller"`
		Stacktrace string    `json:"stacktrace"`
	}

	l := zap.NewLogger(zap.WithLevel(logger.InfoLevel), zap.WithSinks(&b1), zap.WithOutput(zap.JSONOutput))
	l = l.With("key", "value")
	l.Info("test message")

	output1 := b1.String()
	decoder := json.NewDecoder(bytes.NewBuffer([]byte(output1)))
	decoder.DisallowUnknownFields()

	var msg message
	err := decoder.Decode(&msg)
	assert.NoError(t, err)

	assert.Equal(t, logger.InfoLevel, msg.Level)
	assert.Equal(t, "value", msg.Key)
	assert.Equal(t, "test message", msg.Message)
}

func TestZapLogger_ErrorSink(t *testing.T) {
	var b2 bytes.Buffer

	l := zap.NewLogger(zap.WithLevel(logger.InfoLevel), zap.WithSinks(&writer{}), zap.WithOutput(zap.JSONOutput), zap.WithErrorSinks(&b2))
	l = l.With("key", "value")
	l.Error(fmt.Errorf("test error"))

	stderr := b2.String()
	assert.Contains(t, stderr, "write error")
}

func TestZapLogger_ErrorSinks(t *testing.T) {
	var b1 bytes.Buffer
	var b2 bytes.Buffer

	l := zap.NewLogger(zap.WithLevel(logger.InfoLevel), zap.WithSinks(&writer{}), zap.WithOutput(zap.JSONOutput), zap.WithErrorSinks(&b1, &b2))
	l = l.With("key", "value")
	l.Error(fmt.Errorf("test error"))

	stderr1 := b1.String()
	stderr2 := b2.String()
	assert.Equal(t, stderr1, stderr2)
	assert.Contains(t, stderr1, "write error")
}

func TestLogger_EnableDebug(t *testing.T) {
	var buf bytes.Buffer

	l := zap.NewLogger(zap.WithSinks(&buf))

	lvl := l.Level()
	assert.Equal(t, logger.InfoLevel, lvl)

	assert.True(t, l.HasLevel(logger.WarnLevel))
	assert.True(t, l.HasLevel(logger.InfoLevel))
	assert.False(t, l.HasLevel(logger.DebugLevel))

	l.EnableDebug()

	l.Debug("message 1")

	assert.True(t, l.IsDebugEnabled())

	output := buf.String()
	assert.Contains(t, output, "message 1")
}

func TestLogger_Sync(t *testing.T) {
	var buf bytes.Buffer
	ws := zapcore.AddSync(&buf)

	bws := &zapcore.BufferedWriteSyncer{
		WS:            ws,
		Size:          512 * 1024,
		FlushInterval: 60 * time.Minute,
	}

	l := zap.NewLogger(zap.WithWriteSyncer(bws))
	l.Info("test message")

	assert.Equal(t, 0, buf.Len())

	l.Sync()

	assert.NotEqual(t, 0, buf.Len())
}

func TestLogger_WithoutCaller(t *testing.T) {
	for skip, expected := range map[int]string{
		-1: "zap/logger.go:",
		0:  "zap/logger.go:",
		1:  "zap/logger_test.go:",
		2:  "testing/testing.go:",
	} {

		t.Run(fmt.Sprintf("skip %v", skip), func(t *testing.T) {
			var buf bytes.Buffer

			r := &buf
			l := zap.NewLogger(zap.WithSink(r), zap.Skip(skip), zap.WithOutput(zap.JSONOutput))
			l.Info("test message")

			type message struct {
				Key        string
				Level      logger.Level
				Message    string    `json:"msg"`
				Time       time.Time `json:"ts"`
				Caller     string    `json:"caller"`
				Stacktrace string    `json:"stacktrace"`
			}

			output := buf.String()
			decoder := json.NewDecoder(bytes.NewBuffer([]byte(output)))
			decoder.DisallowUnknownFields()

			var msg message
			err := decoder.Decode(&msg)
			assert.NoError(t, err)

			if skip == -1 {
				assert.Empty(t, msg.Caller)
			} else {
				if assert.GreaterOrEqual(t, len(msg.Caller), len(expected)) {
					assert.Contains(t, msg.Caller[:len(expected)], expected)
				}
			}
		})
	}
}

func TestZapLogger_Sampling(t *testing.T) {
	var sampled, dropped int64
	var buf bytes.Buffer

	hook := func(entry zapcore.Entry, dec zapcore.SamplingDecision) {
		if dec == zapcore.LogSampled {
			atomic.AddInt64(&sampled, 1)
		} else {
			atomic.AddInt64(&dropped, 1)
		}
	}

	l := zap.NewLogger(
		zap.WithSink(&buf),
		zap.WithLevel(logger.DebugLevel),
		zap.WithSampling(&zap.SamplingConfig{
			Initial:    1,
			Thereafter: 10,
			Hook:       hook,
		}),
	)

	for i := 0; i < 20; i++ {
		l.Info("test message")
	}

	assert.Equal(t, int64(2), atomic.LoadInt64(&sampled))
	assert.Equal(t, int64(18), atomic.LoadInt64(&dropped))
}

func TestZapLogger_Fields(t *testing.T) {
	var buf bytes.Buffer

	l := zap.NewLogger(
		zap.WithSink(&buf),
		zap.WithLevel(logger.DebugLevel),
		zap.WithField("Test", 99),
	)

	l.Info("test message")

	output := buf.String()
	assert.Contains(t, output, `"Test": 99`)
}
