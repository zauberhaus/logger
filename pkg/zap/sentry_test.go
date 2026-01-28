package zap_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/zauberhaus/logger/pkg/logger"
	"github.com/zauberhaus/logger/pkg/mock"
	hub "github.com/zauberhaus/logger/pkg/sentry"
	"github.com/zauberhaus/logger/pkg/zap"
	gomock "go.uber.org/mock/gomock"
)

func TestWithSentry_Message(t *testing.T) {

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

	var buf bytes.Buffer
	l := zap.NewLogger(zap.WithSentry(sh, logger.InfoLevel), zap.WithSink(&buf))

	tp.EXPECT().SendEvent(gomock.Any()).DoAndReturn(func(event *sentry.Event) {
		assert.Equal(t, "test message", event.Message)
		assert.Equal(t, sentry.LevelInfo, event.Level)
		assert.Empty(t, event.Exception)
	})

	l.Info("test message")
}

func TestWithSentry_Capture(t *testing.T) {

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

	var buf bytes.Buffer
	l := zap.NewLogger(zap.WithSentry(sh, logger.InfoLevel), zap.WithSink(&buf))

	tp.EXPECT().FlushWithContext(gomock.Any())

	tp.EXPECT().SendEvent(gomock.Any()).DoAndReturn(func(event *sentry.Event) {
		assert.Empty(t, event.Message)
		assert.Equal(t, sentry.LevelError, event.Level)
		assert.NotEmpty(t, event.Exception)
		assert.Len(t, event.Exception, 1)

		assert.Equal(t, "*errors.errorString", event.Exception[0].Type)
		assert.Equal(t, "test message", event.Exception[0].Value)
		assert.NotEmpty(t, event.Exception[0].Stacktrace)
	})

	l.Error(fmt.Errorf("test message"))
}
