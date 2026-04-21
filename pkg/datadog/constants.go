package datadog

import "time"

const (
	defaultBatchSize     = 100
	defaultFlushInterval = 5 * time.Second
	maxPayloadBytes      = 5 * 1024 * 1024
)
