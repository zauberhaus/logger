// Copyright 2026 Zauberhaus
// Licensed to Zauberhaus under one or more agreements.
// Zauberhaus licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package sentry

import (
	"github.com/getsentry/sentry-go"
	"github.com/zauberhaus/logger/pkg/logger"
	"github.com/zauberhaus/logger/pkg/zap"
	"go.uber.org/zap/zapcore"
)

func WithSentry(s *sentry.Hub) zap.Option {
	hub := New(s)
	f := func(entry zapcore.Entry) error {
		switch entry.Level {
		case zapcore.WarnLevel:
			hub.Message(logger.WarnLevel, entry.Message)
		}

		return nil
	}

	return zap.OptionFunc(func(o *zap.ZapOptions) {
		o.hub = hub
		o.options = append(o.options, zap.Hooks(f))
	})
}
