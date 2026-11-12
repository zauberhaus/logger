// Copyright 2026 Zauberhaus
// Licensed to Zauberhaus under one or more agreements.
// Zauberhaus licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package zap

import (
	"github.com/getsentry/sentry-go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/zauberhaus/logger/pkg/logger"
	sentry_hub "github.com/zauberhaus/logger/pkg/sentry"
)

func WithSentry(s *sentry.Hub, level logger.Level) Option {
	hub := sentry_hub.New(s)
	f := func(entry zapcore.Entry) error {
		lvl := LogLevel(entry.Level)

		if hub != nil && lvl >= level && lvl != logger.PanicLevel && lvl != logger.FatalLevel && lvl != logger.ErrorLevel {
			hub.Message(lvl, entry.Message)
		}

		return nil
	}

	return OptionFunc(func(o *ZapOptions) {
		o.hub = hub
		o.options = append(o.options, zap.Hooks(f))
	})
}
