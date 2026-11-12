// Copyright 2026 Zauberhaus
// Licensed to Zauberhaus under one or more agreements.
// Zauberhaus licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package datadog

import "time"

const (
	defaultBatchSize     = 100
	defaultFlushInterval = 5 * time.Second
	maxPayloadBytes      = 5 * 1024 * 1024
)
