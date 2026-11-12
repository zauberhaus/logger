// Copyright 2026 Zauberhaus
// Licensed to Zauberhaus under one or more agreements.
// Zauberhaus licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package zap

import (
	"go.uber.org/zap"
)

type (
	Field          = zap.Field
	SamplingConfig = zap.SamplingConfig
	ZapOption      = zap.Option
)

var (
	Any = zap.Any
)
