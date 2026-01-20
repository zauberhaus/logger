package sentry_test

import (
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	gomock "go.uber.org/mock/gomock"

	"github.com/zauberhaus/logger"
	"github.com/zauberhaus/logger/pkg/mock"
	hub "github.com/zauberhaus/logger/pkg/sentry"
)

type textMarshalerImpl struct {
	Value string
}

func (tm textMarshalerImpl) MarshalText() ([]byte, error) {
	return []byte("text:" + tm.Value), nil
}

// TestNew tests the New function
func TestMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tp := mock.NewMockSentryTransport(ctrl)
	tp.EXPECT().Configure(gomock.Any())

	c, err := sentry.NewClient(sentry.ClientOptions{
		Transport: tp,
		Dsn:       "",
	})

	s := sentry.NewScope()

	assert.NoError(t, err)
	assert.NotNil(t, c)

	sh := sentry.NewHub(c, s)

	hub := hub.New(sh)
	assert.NotNil(t, hub)

	testcases := []struct {
		name          string
		level         logger.Level
		expectedLevel sentry.Level
		expected      string
		args          []any
	}{
		{
			"format",
			logger.InfoLevel,
			sentry.LevelInfo,
			"message: test",
			[]any{"message: %v", "test"},
		},
		{
			"slice of strings",
			logger.InfoLevel,
			sentry.LevelInfo,
			"message,test",
			[]any{"message", "test"},
		},
		{
			"duration",
			logger.InfoLevel,
			sentry.LevelInfo,
			"10s",
			[]any{10 * time.Second},
		},
		{
			"text marshaler",
			logger.InfoLevel,
			sentry.LevelInfo,
			"10s,text:test",
			[]any{10 * time.Second, textMarshalerImpl{Value: "test"}},
		},
		{
			"missing args",
			logger.InfoLevel,
			sentry.LevelInfo,
			"",
			[]any{},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.args) > 0 {
				tp.EXPECT().SendEvent(gomock.Any()).DoAndReturn(func(event *sentry.Event) {
					assert.Equal(t, tc.expected, event.Message)
					assert.Equal(t, tc.expectedLevel, event.Level)
					assert.Empty(t, event.Exception)
				})
			} else {
				tp.EXPECT().Flush(gomock.Any()).DoAndReturn(func(timeout time.Duration) bool {
					return true
				})

				tp.EXPECT().SendEvent(gomock.Any()).DoAndReturn(func(event *sentry.Event) {
					assert.Equal(t, tc.expected, event.Message)
					assert.Equal(t, sentry.LevelError, event.Level)
					assert.NotEmpty(t, event.Exception)
				})
			}

			hub.Message(tc.level, tc.args...)
		})
	}
}

func TestMessage_Level(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tp := mock.NewMockSentryTransport(ctrl)
	tp.EXPECT().Configure(gomock.Any())

	c, err := sentry.NewClient(sentry.ClientOptions{
		Transport: tp,
		Dsn:       "",
	})

	s := sentry.NewScope()

	assert.NoError(t, err)
	assert.NotNil(t, c)

	sh := sentry.NewHub(c, s)

	hub := hub.New(sh)
	assert.NotNil(t, hub)

	msg := "test message"

	for level, expected := range map[logger.Level]sentry.Level{
		logger.PanicLevel: sentry.LevelFatal,
		logger.FatalLevel: sentry.LevelFatal,
		logger.ErrorLevel: sentry.LevelError,
		logger.WarnLevel:  sentry.LevelWarning,
		logger.InfoLevel:  sentry.LevelInfo,
		logger.DebugLevel: sentry.LevelDebug,
	} {
		t.Run(level.String(), func(t *testing.T) {
			if expected == sentry.LevelFatal {
				tp.EXPECT().Flush(gomock.Any()).DoAndReturn(func(timeout time.Duration) bool {
					return true
				})
			}

			tp.EXPECT().SendEvent(gomock.Any()).DoAndReturn(func(event *sentry.Event) {
				assert.Equal(t, msg, event.Message)
				assert.Equal(t, expected, event.Level)
				assert.Empty(t, event.Exception)
			})

			hub.Message(level, msg)
		})
	}
}

func message(t *testing.T, tp *mock.MockSentryTransport, hub *hub.Hub, level logger.Level, expectedLevel sentry.Level, expected string, args ...any) {

	if expectedLevel == sentry.LevelFatal {
		tp.EXPECT().Flush(gomock.Any()).DoAndReturn(func(timeout time.Duration) bool {
			return true
		})
	}

	if len(args) > 0 {
		tp.EXPECT().SendEvent(gomock.Any()).DoAndReturn(func(event *sentry.Event) {
			assert.Equal(t, expected, event.Message)
			assert.Equal(t, expectedLevel, event.Level)
			assert.Empty(t, event.Exception)
		})
	} else {
		tp.EXPECT().SendEvent(gomock.Any()).DoAndReturn(func(event *sentry.Event) {
			assert.Equal(t, expected, event.Message)
			assert.Equal(t, sentry.LevelError, event.Level)
			assert.NotEmpty(t, event.Exception)
		})
	}

	hub.Message(level, args...)

}
