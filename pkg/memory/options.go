// Copyright 2026 Zauberhaus
// Licensed to Zauberhaus under one or more agreements.
// Zauberhaus licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package memory

import (
	"github.com/zauberhaus/logger/pkg/zap"
)

const (
	BufferSizeKey = "buffer-size"
	BlockingKey   = "blocking"
)

func WithBufferSize(val int) zap.Option {
	return zap.WithGenericOption(BufferSizeKey, val)
}

func BufferSize(opts ...zap.Option) int {
	size := zap.GetGenericOption[int](BufferSizeKey, opts...)
	if size == 0 {
		size = 1024 * 1024
	}

	return size
}

func WithBlocking(val bool) zap.Option {
	return zap.WithGenericOption(BlockingKey, val)
}

func Blocking(opts ...zap.Option) bool {
	return zap.GetGenericOption[bool](BlockingKey, opts...)
}
